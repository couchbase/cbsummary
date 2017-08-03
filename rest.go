package main

//
// cbsummary - a command-line utility for creating a summary report for a set of clusters
//

import (
	"crypto/tls"
	"crypto/x509"
    "encoding/json"
    "fmt"
    "io/ioutil"
    "net/http"
    "net/url"
   	"strings"
)

// types for communicating with the server

type RestClientError struct {
	method string
	url    string
	err    error
}

func (e RestClientError) Error() string {
	return fmt.Sprintf("Rest client error (%s %s): %s", e.method, e.url, e.err)
}

type ServiceNotAvailableError struct {
	service string
}

func (e ServiceNotAvailableError) Error() string {
	return fmt.Sprintf("Service `%s` is not available on target cluster", e.service)
}

type SSLNotAvailableError struct {
	service string
}

func (e SSLNotAvailableError) Error() string {
	return fmt.Sprintf("SSL is not available for `%s` on target cluster", e.service)
}

type UnknownAuthorityError struct {
	err error
}

func (e UnknownAuthorityError) Error() string {
	return fmt.Sprintf("%s\n\nIf you are using self-signed certificates you can "+
		"re-run this command with\nthe --no-ssl-verify flag. Note however that"+
		" disabling ssl verification\nmeans that cbbackupmgr will be vulnerable"+
		" to man-in-the-middle attacks.\n\nFor the most secure access to Couchbase"+
		" make sure that you have X.509\ncertificates set up in your cluster and"+
		" use the --cacert flag to specify\nyour client certificate.",
		e.err.Error())
}

type HttpError struct {
	code     int
	method   string
	resource string
	body     string
}

func (e HttpError) Error() string {
	switch e.code {
	case http.StatusBadRequest:
		return fmt.Sprintf("Bad request executing %s %s due to %s",
			e.method, e.resource, e.body)
	case http.StatusUnauthorized:
		return fmt.Sprintf("Authentication error executing \"%s %s\" "+
			"check username and password", e.method, e.resource)
	case http.StatusForbidden:
		return e.body
	case http.StatusInternalServerError:
		return fmt.Sprintf("Internal server error while executing \"%s %s\" "+
			"check the server logs for more details", e.method, e.resource)
	default:
		return fmt.Sprintf("Recieved error %d while executing \"%s %s\"",
			e.code, e.method, e.resource)
	}
}

func (e HttpError) Code() int {
	return e.code
}

type RestClient struct {
	client   http.Client
	secure   bool
	host     string
	username string
	password string
}

func CreateRestClient(host, username, password string, tlsConfig *tls.Config) *RestClient {
	tr := &http.Transport{TLSClientConfig: tlsConfig}
	return &RestClient{
		client:   http.Client{Transport: tr},
		secure:   strings.HasPrefix(host, "https://"),
		host:     host,
		username: username,
		password: password,
	}
}

// types for parsing the JSON in the config file

type Cluster struct {
	Login string `json:"login"`
	Pass string `json:"pass"`
	Nodes []string `json:"nodes"`
}

type ClusterList struct {
    Clusters []Cluster `json:"clusters"`
}

//
// types for parsing JSON from /pools REST API
//

type ComponentsVersion struct {
    Ale string `json:"ale"`
    Asn1 string `json:"asn1"`
    Crypto string `json:"crypto"`
    Inets string `json:"inets"`
    Kernel string `json:"keynel"`
    Lhttpc string `json:"lhttpc"`
    Ns_server string `json:"ns_server"`
    Os_mon string `json:"os_mon"`
    Public_key string `json:"public_key"`
    Sasl string `json:"sasl"`
    Ssl string `json:"ssl"`
    Stdlib string `json:"stdlib"`
}

type Pools struct {
    Components ComponentsVersion `json:"componentsVersion"`
    ImplementationVersion string `json:"implementationVersion"`
    IsEnterprise bool `json:"isEnterprise"`
    Uuid string `json:"uuid"`
}


type PoolsDefault struct {
    Alerts []json.RawMessage `json:"alerts"`
    Balanced bool  `json:"balanced"`
    ClusterName string `json:"clusterName"`
    FtsMemoryQuota int `json:"ftsMemoryQuota"`
    IndexMemoryQuota int `json:"indexMemoryQuota"`
    MemoryQuota int `json:"memoryQuota"`
    Name string `json:"name"`
    Nodes []NodeInfo `json:"nodes"`
    RebalanceStatus string `json:"rebalanceStatus"`
    StorageTotals ClusterStorageInfo `json:"storageTotals"`
}

type NodeInfo struct {
    ClusterMembership string `json:"clusterMembership"`
    Hostname string `json:"hostname"`
    InterestingStats NodeStats `json:"interestingStats"`
    McdMemoryAllocated float64 `json:"mcdMemoryAllocated"`
    McdMemoryReserved float64 `json:"mcdMemoryReserved"`
    MemoryFree float64 `json:"memoryFree"`
    MemoryTotal float64 `json:"memoryTotal"`
    OS string `json:"os"`
    Services []string `json:"services"`
    Status string `json:"status"`
    SystemStats SysStats `json:"systemStats"`
    Uptime string `json:"uptime"`
    Version string `json:"version"`
}

type NodeStats struct {
    Cmd_get float64 `json:"cmd_get"`
    Couch_docs_actual_disk_size float64 `json:"couch_docs_actual_disk_size"`
    Couch_docs_data_size float64 `json:"couch_docs_data_size"`
    Couch_spatial_data_size float64 `json:"couch_spatial_data_size"`
    Couch_spatial_disk_size float64 `json:"couch_spatial_disk_size"`
    Couch_views_actual_disk_size float64 `json:"couch_views_actual_disk_size"`
    Couch_views_data_size float64 `json:"couch_views_data_size"`
    Curr_items float64 `json:"curr_items"`
    Curr_items_tot float64 `json:"curr_items_tot"`
    Ep_bg_fetched float64 `json:"ep_bg_fetched"`
    Get_hits float64 `json:"get_hits"`
    Mem_used float64 `json:"mem_used"`
    Ops float64 `json:"ops"`
    Vb_active_num_non_resident float64 `json:"vb_active_num_non_resident"`
    Vb_replica_curr_items float64 `json:"vb_replica_curr_items"`
}

type SysStats struct {
    Cpu_utilization_rate float64 `json:"cpu_utilization_rate"`
    Mem_free float64 `json:"mem_free"`
    Mem_total float64 `json:"mem_total"`
    Swap_total float64 `json:"swap_total"`
    Swap_used float64 `json:"swap_used"`
}

type ClusterStorageInfo struct {
    HDD HDDStorageInfo `json:"hdd"`
    RAM RAMStorageInfo `json:"ram"`
}

type HDDStorageInfo struct {
    Free float64 `json:"free"`
    QuotaTotal float64 `json:""`
    Total float64 `json:"total"`
    Used float64 `json:"used"`
    UsedByData float64 `json:"usedByData"`
}

type RAMStorageInfo struct {
    QuotaTotal float64 `json:"quotaTotal"`
    QuotaTotalPerNode float64 `json:"quotaTotalPerNode"`
    QuotaUsed float64 `json:"quotaUsed"`
    QuotaUsedPerNode float64 `json:"quataUsedPerNode"`
    Total float64 `json:"total"`
    Used float64 `json:"used"`
    UsedByData float64 `json:"usedByData"`
}


////////////////////////////////////////////////////////////////////////////

// type for output

type ClusterSummary struct {
    ImplementationVersion string `json:"implementationVersion"`
    IsEnterprise bool `json:"isEnterprise"`
    Uuid string `json:"uuid"`
    Balanced bool `json:"balanced"`
    ClusterName string `json:"clusterName"`
    FtsMemoryQuota int `json:"ftsMemoryQuota"`
    IndexMemoryQuota int `json:"indexMemoryQuota"`
    MemoryQuota int `json:"memoryQuota"`
    Name string `json:"name"`
    NodeCount int `json:"nodeCount"`
    NodeVersions map[string]int `json:"nodeVersions"`
    Nodes []NodeInfo `json:"nodes"`
    RebalanceStatus string `json:"rebalanceStatus"`
    StorageTotals ClusterStorageInfo `json:"storageTotals"`
}


////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////

func (r *RestClient) executeGet(uri string) (*http.Response, error) {
	method := "GET"
	req, err := http.NewRequest(method, uri, nil)
	if err != nil {
		return nil, &RestClientError{method, uri, err}
	}
	req.SetBasicAuth(r.username, r.password)

	resp, err := r.executeRequest(req)
	if err != nil {
		return nil, err
	}

	return resp, nil
}


func (r *RestClient) executeRequest(req *http.Request) (*http.Response, error) {
	resp, err := r.client.Do(req)
	if err != nil {
		switch err.(type) {
		case *url.Error:
			return nil, err.(*url.Error).Err
		case x509.UnknownAuthorityError:
			return nil, UnknownAuthorityError{err}
		case x509.CertificateInvalidError:
			return nil, err
		case x509.ConstraintViolationError:
			return nil, err
		case x509.HostnameError:
			return nil, err
		case x509.SystemRootsError:
			return nil, err
		case x509.UnhandledCriticalExtension:
			return nil, err
		default:
			return nil, &RestClientError{req.Method, req.URL.String(), err}
		}
	}

	//clog.Log("(Rest) %s %s %d", req.Method, req.URL.String(), resp.StatusCode)
	if resp.StatusCode == http.StatusBadRequest {
		contents, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			contents = []byte("<no body>")
		}
		resp.Body.Close()
		return nil, HttpError{resp.StatusCode, req.Method, req.URL.String(), string(contents)}
	} else if resp.StatusCode == http.StatusForbidden {
		type overlay struct {
			Message     string
			Permissions []string
		}

		var data overlay
		decoder := json.NewDecoder(resp.Body)
		decoder.UseNumber()
		err = decoder.Decode(&data)
		if err != nil {
			return nil, HttpError{resp.StatusCode, req.Method, req.URL.String(), ""}
		}

		msg := data.Message + ": " + strings.Join(data.Permissions, ", ")
		return nil, HttpError{resp.StatusCode, req.Method, req.URL.String(), msg}
	} else if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusCreated {
		return nil, HttpError{resp.StatusCode, req.Method, req.URL.String(), ""}
	}

	return resp, nil
}

//
// for each cluster, we call the /pools REST API to get:
// - componentsVersion
// - implementationVersion as version
// - isEnterprise as isEnterpriseEdition
// - uuid

func (r *RestClient) GetPoolsData() (*Pools, error) {
	url := r.host + "/pools"
	resp, err := r.executeGet(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data Pools
	decoder := json.NewDecoder(resp.Body)
	decoder.UseNumber()
	err = decoder.Decode(&data)
	if err != nil {
		return nil, &RestClientError{"GET", url, err}
	}

	return &data, nil
}


// for each cluster, we call the /pools REST API to get:
// - componentsVersion
// - implementationVersion as version
// - isEnterprise as isEnterpriseEdition
// - uuid

type ResultMap map[string]*json.RawMessage

//func (r *RestClient) GetPoolsDefaultData() (*ResultMap, error) {
func (r *RestClient) GetPoolsDefaultData() (*PoolsDefault, error) {
	url := r.host + "/pools/default"
	resp, err := r.executeGet(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var resultMap PoolsDefault
	//var resultMap ResultMap
	decoder := json.NewDecoder(resp.Body)
	decoder.UseNumber()
	err = decoder.Decode(&resultMap)
	if err != nil {
		return nil, &RestClientError{"GET", url, err}
	}

	return &resultMap, nil
}

