/*
Copyright 2017-Present Couchbase, Inc.

Use of this software is governed by the Business Source License included in
the file licenses/BSL-Couchbase.txt.  As of the Change Date specified in that
file, in accordance with the Business Source License, use of this software will
be governed by the Apache License, Version 2.0, included in the file
licenses/APL2.txt.
*/

package main

//
// cbsummary - a command-line utility for creating a summary report for a set of clusters
//

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"strings"
	"time"
)

// data type for holding cluster info

// count of buckets of different types
type BucketSummary struct {
	Emphemeral int `json:"ephemeral"`
	Membase    int `json:"membase"`
	Memcached  int `json:"memcached"`
	Total      int `json:"total"`
}

// cluster settings
type ClusterSettings struct {
	//Compaction CompactionSettings `json:"compaction"`
	EnableAutoFailover bool   `json:"enable_auto_failover"`
	FailoverTimeout    int    `json:"failover_timeout"`
	IndexStorageMode   string `json:"index_storage_mode"`
}

// types for ODP reports
type BriefCluster struct {
	Nodes []BriefNode `json:"nodes"`
	Size  int         `json:"cluster_size"`
	UUID  string      `json:"cluster_uuid"`
}

type BriefNode struct {
	Cores   float64 `json:"cpu_cores_available"`
	RAM     float64 `json:"mem_total"`
	Name    string  `json:"hostname"`
	Version string  `json:"version"`
}

type ClusterInfo struct {
	AdminAuditEnabled bool            `json:"adminAuditEnabled"`
	AdminLDAPEnabled  bool            `json:"adminLDAPEnabled"`
	Buckets           BucketSummary   `json:"buckets"`
	Cluster_Settings  ClusterSettings `json:"cluester_settings"`
}

type SummaryInfo struct {
	NumClusters   int            `json:"#clusters"`
	TotalNumNodes int            `json:"#nodes"`
	NodeVersions  map[string]int `json:"#nodeVersions"`
	Clusters      []interface{}  `json:"clusters"`
}

type ClusterError struct {
	TheCluster Cluster `json:"error_with_cluster"`
	ErrMsg     string  `json:"error_message"`
}

// flags for the command-line

var CONFIG_FILE = flag.String("config", "", "Config file listing clusters and credentials to summarize.")
var OUTPUT_FILE = flag.String("output", "", "Name for output file (default cbsummary.out.<timestamp>).")
var HELP = flag.Bool("help", false, "Print a help message.")
var FULL = flag.Bool("full", false, "Produce an extensive report, instead of just core and RAM usage.")
var CSV = flag.Bool("csv", false, "Produce a report in CSV format. Not compatible with full reports.")

func main() {
	flag.Parse()

	// help message
	if *HELP || len(*CONFIG_FILE) == 0 {
		fmt.Printf("usage: cbsummary --config=<config file> [--output=<output file>] [--full]\n\n")
		fmt.Printf("  cbsummary connects to a set of Couchbase clusters and generates a summary report.\n\n")
		fmt.Printf("  The config file contains JSON specifying an array of information on each cluster,\n")
		fmt.Printf("  giving the Couchbase login/password and one or more IP addresses for cluster nodes.\n")
		fmt.Printf("  An example config file giving information about 2 clusters is:\n\n")
		fmt.Printf("  { \"clusters\": [\n")
		fmt.Printf("    {\"login\": \"Administrator\", \"pass\": \"password1\", \"nodes\": [\"http://192.168.1.1:8091\"]},\n")
		fmt.Printf("    {\"login\": \"Administrator\", \"pass\": \"password2\", \"nodes\": [\"http://192.166.1.1:8091\",\"http://192.16.1.2:8091\"]}\n")
		fmt.Printf("  ]}\n\n")
		fmt.Printf("  The default report format includes RAM and Core utilization across each specified cluster,\n")
		fmt.Printf("  since that information is useful in determining compliance with Couchbase licenses. If you\n")
		fmt.Printf("  specify --csv, then the report is generated in CSV instead of JSON. If, instead, you\n")
		fmt.Printf("  specify --full, then a much more detailed report is generated.\n\n")
		fmt.Printf("  The summary report is sent to the file 'cbsummary.out.<timestamp>', unless a different\n")
		fmt.Printf("  file name is specified with the --output option.\n\n")
		return
	}

	// can't have both FULL and CSV
	if *FULL && *CSV {
		fmt.Printf("CSV format is not available for full reports.\n\n")
		return
	}

	// need some configuration
	if CONFIG_FILE == nil || len(*CONFIG_FILE) == 0 {
		fmt.Printf("You must specify a configuration file.\n\n")
		return
	}

	var output_file string
	if OUTPUT_FILE == nil || len(*OUTPUT_FILE) == 0 {
		now := time.Now()
		output_file = fmt.Sprintf("cbsummary.out.%04d-%02d-%02d-%02d:%02d:%02d", now.Year(), now.Month(), now.Day(),
			now.Hour(), now.Minute(), now.Second())
	} else {
		output_file = *OUTPUT_FILE
	}

	// load the configuration

	config, err := ioutil.ReadFile(*CONFIG_FILE)
	if err != nil {
		fmt.Printf("Error reading configuration file %s: %s\n\n", *CONFIG_FILE, err)
		return
	}

	// parse the configuration as JSON
	var clusters ClusterList
	err = json.Unmarshal(config, &clusters)
	if err != nil {
		fmt.Printf("Error parsing configuration file %s: %s\n\n", *CONFIG_FILE, err)
		return
	}

	fmt.Printf("Working from config file: %s\n", *CONFIG_FILE)

	clusterSummary := new(SummaryInfo)
	clusterSummary.NumClusters = len(clusters.Clusters)
	clusterSummary.TotalNumNodes = 0
	clusterSummary.NodeVersions = make(map[string]int)
	clusterSummary.Clusters = make([]interface{}, len(clusters.Clusters))

	// loop through the clusters
	for cnum, cluster := range clusters.Clusters {
		//fmt.Printf("\n\nCluster login: %s pass %s nodes: %v\n", cluster.Login, cluster.Pass, cluster.Nodes)
		var thisCluster *ClusterSummary
		var briefCluster *BriefCluster
		var cerr error

		for _, node := range cluster.Nodes {
			client := CreateRestClient(node, cluster.Login, cluster.Pass, nil)

			// get /pools and /pools/defaults
			pools, err := client.GetPoolsData()
			if err != nil {
				cerr = err
				fmt.Printf("Error getting bucket settings from node %s: %v\n", node, err)
				continue // try the next node
			}

			poolsDefaults, err := client.GetPoolsDefaultData()

			if err != nil {
				cerr = err
				fmt.Printf("Error getting pools/default from node %s: %v\n", node, err)
				continue // try the next node
			}

			// if we make it this far, we have both /pools and /pools/defaults

			// full report? get all details

			if *FULL {
				thisCluster = new(ClusterSummary)
				thisCluster.ImplementationVersion = pools.ImplementationVersion
				thisCluster.IsEnterprise = pools.IsEnterprise
				thisCluster.Uuid = pools.Uuid

				thisCluster.Balanced = poolsDefaults.Balanced
				thisCluster.ClusterName = poolsDefaults.ClusterName
				thisCluster.FtsMemoryQuota = poolsDefaults.FtsMemoryQuota
				thisCluster.IndexMemoryQuota = poolsDefaults.IndexMemoryQuota
				thisCluster.MemoryQuota = poolsDefaults.MemoryQuota
				thisCluster.Name = poolsDefaults.Name
				thisCluster.NodeCount = len(poolsDefaults.Nodes)
				thisCluster.Nodes = poolsDefaults.Nodes
				thisCluster.RebalanceStatus = poolsDefaults.RebalanceStatus
				thisCluster.StorageTotals = poolsDefaults.StorageTotals

				// for each of the nodes in this cluster, show the distribution of versions
				nodeVersions := make(map[string]int)
				for _, nodeInfo := range poolsDefaults.Nodes {
					nodeVersions[nodeInfo.Version] = nodeVersions[nodeInfo.Version] + 1
					clusterSummary.NodeVersions[nodeInfo.Version] = clusterSummary.NodeVersions[nodeInfo.Version] + 1
				}
				thisCluster.NodeVersions = nodeVersions

				clusterSummary.Clusters[cnum] = thisCluster
				clusterSummary.TotalNumNodes = clusterSummary.TotalNumNodes + len(poolsDefaults.Nodes)

			} else {
				// for a partial report, get the cluster_size, uuid, and an array of nodes with:
				// - cpu cores
				// - hostname
				// - memory limit

				briefCluster = new(BriefCluster)

				nodes := make([]BriefNode, len(poolsDefaults.Nodes))
				curNode := 0
				for _, nodeInfo := range poolsDefaults.Nodes {
					node := new(BriefNode)
					node.Cores = nodeInfo.SystemStats.CPU_cores_available
					node.RAM = nodeInfo.MemoryTotal / 1024.0 / 1024.0 / 1024.0
					node.Name = nodeInfo.Hostname
					node.Version = nodeInfo.Version
					nodes[curNode] = *node
					curNode = curNode + 1
				}

				briefCluster.Nodes = nodes
				briefCluster.Size = len(nodes)
				briefCluster.UUID = pools.Uuid

				clusterSummary.Clusters[cnum] = briefCluster

				clusterSummary.TotalNumNodes = clusterSummary.TotalNumNodes + len(poolsDefaults.Nodes)

				// for each of the nodes in this cluster, show the distribution of versions
				for _, nodeInfo := range poolsDefaults.Nodes {
					clusterSummary.NodeVersions[nodeInfo.Version] = clusterSummary.NodeVersions[nodeInfo.Version] + 1
				}
			}

			//  debugging output
			//body, err := json.Marshal(clusterSummary.Clusters[cnum])
			//if (err == nil) {
			//    fmt.Printf("%s\n\n",string(body))
			//}

			// when we've gotten all the info, break from this look to look at the next cluster

			break
		}

		// if we get this far with thisCluster unset, we need to replace it with a
		// different item indicating the error.

		if thisCluster == nil && briefCluster == nil {
			//fmt.Printf("Failed to contact cluster, error: %v\n",cerr)
			errorStatus := new(ClusterError)
			errorStatus.TheCluster = cluster
			if cerr != nil {
				errorStatus.ErrMsg = cerr.Error()
			} else {
				errorStatus.ErrMsg = "Unknown Error"
			}
			clusterSummary.Clusters[cnum] = errorStatus
		}
	}

	// create the output, either JSON or CSV

	var body []byte

	if *CSV {
		var buffer strings.Builder
		buffer.WriteString("cluster_num\tcluster_uuid\tcluster_size\thostname\tcpu_cores\tRAM\n")

		for cnum, icluster := range clusterSummary.Clusters {
			cluster, ok := icluster.(*BriefCluster)
			if ok {
				for _, node := range cluster.Nodes {
					// no cores info for earlier than 6.5
					if node.Version < "6.5" {
						buffer.WriteString(fmt.Sprintf("%d\t%s\t%d\t%s\tN/A\t%.1f\n", cnum, cluster.UUID, cluster.Size,
							node.Name, node.RAM))
					} else {
						buffer.WriteString(fmt.Sprintf("%d\t%s\t%d\t%s\t%.1f\t%.1f\n", cnum, cluster.UUID, cluster.Size,
							node.Name, node.Cores, node.RAM))
					}
				}
			}
		}
		body = []byte(buffer.String())

	} else { // JSON output
		body, err = json.MarshalIndent(clusterSummary, "", "  ")
		if err != nil {
			fmt.Printf("Error marshalling summary: %v\n", err)
			return
		}
	}

	err = ioutil.WriteFile(output_file, body, 0644)
	if err != nil {
		fmt.Printf("Error writing output file %s: %v\n", output_file, err)
		return
	}

	fmt.Printf("Wrote information on %d clusters to file %s.\n", clusterSummary.NumClusters, output_file)
}
