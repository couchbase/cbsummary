package main

//
// cbsummary - a command-line utility for creating a summary report for a set of clusters
//

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
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

type ClusterInfo struct {
	AdminAuditEnabled bool            `json:"adminAuditEnabled"`
	AdminLDAPEnabled  bool            `json:"adminLDAPEnabled"`
	Buckets           BucketSummary   `json:"buckets"`
	Cluster_Settings  ClusterSettings `json:"cluester_settings"`
}

type SummaryInfo struct {
	NumClusters   int              `json:"#clusters"`
	TotalNumNodes int              `json:"#nodes"`
	NodeVersions  map[string]int   `json:"#nodeVersions"`
	Clusters      []ClusterSummary `json:"clusters"`
}

// flags for the command-line

var CONFIG_FILE = flag.String("config", "", "Config file listing clusters and credentials to summarize.")
var OUTPUT_FILE = flag.String("output", "", "Name for output file (default cbsummary.out.<timestamp>).")

func main() {
	flag.Parse()

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
	fmt.Printf("Using config file: %s\n", *CONFIG_FILE)
	config, err := ioutil.ReadFile(*CONFIG_FILE)
	if err != nil {
		fmt.Printf("Error reading configuration: %s\n\n", err)
		return
	}

	// parse the configuration as JSON
	var clusters ClusterList
	err = json.Unmarshal(config, &clusters)
	if err != nil {
		fmt.Printf("Error parsing configuration: %s\n\n", err)
		return
	}

	clusterSummary := new(SummaryInfo)
	clusterSummary.NumClusters = len(clusters.Clusters)
	clusterSummary.TotalNumNodes = 0
	clusterSummary.NodeVersions = make(map[string]int)
	clusterSummary.Clusters = make([]ClusterSummary, len(clusters.Clusters))

	// loop through the clusters
	for cnum, cluster := range clusters.Clusters {
		//fmt.Printf("\n\nCluster login: %s pass %s nodes: %v\n", cluster.Login, cluster.Pass, cluster.Nodes)

		for _, node := range cluster.Nodes {
			client := CreateRestClient(node, cluster.Login, cluster.Pass, nil)

			// get /pools and /pools/defaults
			pools, err := client.GetPoolsData()
			if err != nil {
				fmt.Printf("Error getting bucket settings from node %s: %v\n", node, err)
				continue // try the next node
			}

			poolsDefaults, err := client.GetPoolsDefaultData()

			if err != nil {
				fmt.Printf("Error getting pools/default from node %s: %v\n", node, err)
				continue // try the next node
			}

			// if we make it this far, we have both /pools and /pools/defaults

			clusterSummary.Clusters[cnum].ImplementationVersion = pools.ImplementationVersion
			clusterSummary.Clusters[cnum].IsEnterprise = pools.IsEnterprise
			clusterSummary.Clusters[cnum].Uuid = pools.Uuid

			clusterSummary.Clusters[cnum].Balanced = poolsDefaults.Balanced
			clusterSummary.Clusters[cnum].ClusterName = poolsDefaults.ClusterName
			clusterSummary.Clusters[cnum].FtsMemoryQuota = poolsDefaults.FtsMemoryQuota
			clusterSummary.Clusters[cnum].IndexMemoryQuota = poolsDefaults.IndexMemoryQuota
			clusterSummary.Clusters[cnum].MemoryQuota = poolsDefaults.MemoryQuota
			clusterSummary.Clusters[cnum].Name = poolsDefaults.Name
			clusterSummary.Clusters[cnum].NodeCount = len(poolsDefaults.Nodes)
			clusterSummary.TotalNumNodes = clusterSummary.TotalNumNodes + len(poolsDefaults.Nodes)
			clusterSummary.Clusters[cnum].Nodes = poolsDefaults.Nodes
			clusterSummary.Clusters[cnum].RebalanceStatus = poolsDefaults.RebalanceStatus
			clusterSummary.Clusters[cnum].StorageTotals = poolsDefaults.StorageTotals

			// for each of the nodes in this cluster, show the distribution of versions
			nodeVersions := make(map[string]int)
			for _, nodeInfo := range poolsDefaults.Nodes {
				nodeVersions[nodeInfo.Version] = nodeVersions[nodeInfo.Version] + 1
				clusterSummary.NodeVersions[nodeInfo.Version] = clusterSummary.NodeVersions[nodeInfo.Version] + 1
			}
			clusterSummary.Clusters[cnum].NodeVersions = nodeVersions

			//  debugging output
			//body, err := json.Marshal(clusterSummary.Clusters[cnum])
			//if (err == nil) {
			//    fmt.Printf("%s\n\n",string(body))
			//}

			// when we've gotten all the info, break from this look to look at the next cluster

			break
		}
	}

	body, err := json.MarshalIndent(clusterSummary,"","  ")
	if err != nil {
		fmt.Printf("Error marshalling summary: %v\n", err)
		return
	}

	err = ioutil.WriteFile(output_file,body,0644)
	if err != nil {
		fmt.Printf("Error writing output file %s: %v\n", output_file,err)
		return
	}

    fmt.Printf("Wrote information on %d clusters to file %s.\n",clusterSummary.NumClusters,output_file)
}
