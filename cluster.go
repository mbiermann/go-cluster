// The cluster package provides an interface to access a distributed backend via HTTP requests 
// based on the standard http package
package cluster

import(
	"net/http"
	"sync"
	"fmt"
	"regexp"
	"errors"
	"math/rand"
	"time"
)

type ClusterConfig struct {
	Hosts 						  	[]string
	NodeReanimationAfterSeconds 	int64
}

func(config *ClusterConfig) UnsupportedNodes(nodes []*Node) []*Node {
	unsupportedNodes := []*Node{}
	for _, node := range nodes {
		found := false
		for _, host := range config.Hosts {
			if host == node.Host {
				found = true
				break
			}
		}
		if !found {
			unsupportedNodes = append(unsupportedNodes, node)
		}
	}
	return unsupportedNodes
} 	

func(config *ClusterConfig) SupportedNodesMissing(nodes []*Node) []*Node {
	supportedNodesMissing := []*Node{}
	for _, host := range config.Hosts {
		found := false
		for _, node := range nodes {	
			if host == node.Host {
				found = true
				break
			}
		}
		if !found {
			supportedNodesMissing = append(supportedNodesMissing, NewNode(host))
		}
	}
	return supportedNodesMissing
}

func RemoveNode(nodes []*Node, nodeToRemove *Node) []*Node {
	for idx, node := range nodes {
		if node == nodeToRemove {
			nodes = append(nodes[:idx], nodes[idx+1:] ...)
			break
		}
	}
	return nodes
}

func AddNodes(nodes, nodesToAdd []*Node) []*Node {
	for _, nodeToAdd := range nodesToAdd {
		nodes = AddNode(nodes, nodeToAdd)
	}
	return nodes
}

func AddNode(nodes []*Node, nodeToAdd *Node) []*Node {
	found := false
	for _, node := range nodes {
		if nodeToAdd == node {
			found = true
		}
	}
	if !found {
		nodes = append(nodes, nodeToAdd)
	}
	return nodes
}

type Node struct {
	Client 	*http.Client
	Host 	string
}

func(node *Node) Do(req *http.Request) (resp *http.Response, err error) {
	// Set the scheme and host of the request
	req.URL.Scheme = "http"
	req.URL.Host = node.Host
	// Verify the request header contains the keep-alive directive to keep up the connection for 
	// re-use where possible
	if req.Header == nil {
		req.Header = map[string][]string{}
	}
	req.Header["Connection"] = []string{"keep-alive"}
	resp, err = node.Client.Do(req)
	return
}

func NewNode(host string) *Node {
	return &Node{Host: host, Client: &http.Client{}}
}

type Cluster struct {
	http.Client
	Config 			ClusterConfig
	Nodes 			[]*Node
	NodesMutex 		*sync.RWMutex
	DeadPool 		[]*Node
	DeadPoolMutex	*sync.RWMutex
	NodeReanimationAfterSeconds int64
}

func MatchString(pattern, str string) bool {
	matched, _ := regexp.MatchString(pattern, str)
	return matched
}

func(cluster *Cluster) Do(req *http.Request) (resp *http.Response, err error) {
	if len(cluster.Nodes) == 0 {
		err = errors.New("No cluster nodes available")
		return
	}
	cluster.NodesMutex.Lock()
	rand.Seed(time.Now().Unix())
    idx := rand.Intn(len(cluster.Nodes))
	node := cluster.Nodes[idx]
	cluster.NodesMutex.Unlock()
	resp, err = node.Do(req)
	errMsg := fmt.Sprintf("%v", err)
	if MatchString("connection refused", errMsg) || MatchString("no route to host", errMsg) || MatchString("invalid port", errMsg) {
		cluster.NodesMutex.Lock()
		cluster.Nodes = RemoveNode(cluster.Nodes, node)
		cluster.NodesMutex.Unlock()
		cluster.DeadPoolMutex.Lock()
		cluster.DeadPool = AddNode(cluster.DeadPool, node)
		cluster.DeadPoolMutex.Unlock()
		if cluster.NodeReanimationAfterSeconds > 0 {
			go func(){
				time.Sleep(time.Duration(cluster.NodeReanimationAfterSeconds * 1000 * 1000 * 1000))
				cluster.DeadPoolMutex.Lock()
				cluster.DeadPool = RemoveNode(cluster.DeadPool, node)
				cluster.DeadPoolMutex.Unlock()
				cluster.NodesMutex.Lock()
				cluster.Nodes = AddNode(cluster.Nodes, node)
				cluster.NodesMutex.Unlock()
			}()
		}
		resp, err = cluster.Do(req)
	}
	return
}

func(cluster *Cluster) UpdateWithConfig(config *ClusterConfig) {
	cluster.NodesMutex.Lock()
	cluster.DeadPoolMutex.Lock()
	// Remove any non-supported nodes from the cluster
	allNodes := append(cluster.DeadPool, cluster.Nodes ...)
	for _, node := range config.UnsupportedNodes(allNodes) {
		cluster.DeadPool = RemoveNode(cluster.DeadPool, node)
		cluster.Nodes = RemoveNode(cluster.Nodes, node)
	}
	// Add any newly supported node to the cluster
	cluster.Nodes = AddNodes(cluster.Nodes, config.SupportedNodesMissing(allNodes))
	cluster.DeadPoolMutex.Unlock()
	cluster.NodesMutex.Unlock()
	cluster.NodeReanimationAfterSeconds = config.NodeReanimationAfterSeconds
}

func NewCluster(config *ClusterConfig) (cluster *Cluster, err error) {
	c := &Cluster{}
	c.NodesMutex = &sync.RWMutex{}
	c.DeadPoolMutex = &sync.RWMutex{}
	c.UpdateWithConfig(config)
	cluster = c
	return
}