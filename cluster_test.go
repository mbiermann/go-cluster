package cluster

import (
	"testing"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"fmt"
	"strings"
	"net/url"
	"time"
)

type HTTPHandler func(w http.ResponseWriter, r *http.Request)

func NewHandler(t *testing.T) HTTPHandler {
	return func (w http.ResponseWriter, r *http.Request) {
		t.Logf("--> Test Server received request %v on port %s", r, strings.Split(r.Host, ":")[1])
		fmt.Fprintf(w, strings.Split(r.Host, ":")[1])	
	}
}

func TestClusterForwardsToOneOfOneEndpoints(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(NewHandler(t)))
	defer ts.Close()
	port := strings.Split(ts.URL, ":")[2]
	config := &ClusterConfig{Hosts: []string{"localhost:"+port}}
	cluster, err := NewCluster(config)
	if err != nil {
		t.Fatalf("Unexpected error when create cluster with config `%v`: %v", config, err)
		return
	}
	u := &url.URL{Path: "/"}
	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		t.Fatalf("Unexpected error when create cluster request: %v", err)
		return
	}
	resp, err := cluster.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		t.Fatalf("Cluster client on Get request raised error: %v", err)
		return
	}
	buf, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Reading cluster Get request response raised error: %v", err)
		return
	}
	respondedPort := string(buf)
	if port != respondedPort {
		t.Fatalf("Expected responded port %s to equal server port %s", respondedPort, port)
		return	
	}
}

func TestClusterForwardsToRandomOfMutlipleEndpoints(t *testing.T) {
	var ports []string
	var hosts []string
	for i := 0; i<=3; i++ {
		ts := httptest.NewServer(http.HandlerFunc(NewHandler(t)))
		defer ts.Close()
		port := strings.Split(ts.URL, ":")[2]
		ports = append(ports, port)
		hosts = append(hosts, "localhost:"+port)
	}
	t.Logf("--> Ports used in test cluster: %v", ports)
	config := &ClusterConfig{Hosts: hosts}
	cluster, err := NewCluster(config)
	if err != nil {
		t.Fatalf("Unexpected error when create cluster with config `%v`: %v", config, err)
		return
	}
	u := &url.URL{Path: "/"}
	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		t.Fatalf("Unexpected error when create cluster request: %v", err)
		return
	}
	resp, err := cluster.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		t.Fatalf("Cluster client on Get request raised error: %v", err)
		return
	}
}

func TestClusterRecognizesDeadEnds(t *testing.T) {
	var ports []string
	var hosts []string
	for i := 0; i<=0; i++ {
		ts := httptest.NewServer(http.HandlerFunc(NewHandler(t)))
		defer ts.Close()
		port := strings.Split(ts.URL, ":")[2]
		ports = append(ports, port)
		hosts = append(hosts, "localhost:"+port)
	}
	ports = append(ports, "43892")
	hosts = append(hosts, "localhost:43892")
	t.Logf("--> Ports used in test cluster: %v", ports)
	config := &ClusterConfig{Hosts: hosts, NodeReanimationAfterSeconds: 1}
	cluster, err := NewCluster(config)
	if err != nil {
		t.Fatalf("Unexpected error when create cluster with config `%v`: %v", config, err)
		return
	}
	u := &url.URL{Path: "/"}
	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		t.Fatalf("Unexpected error when create cluster request: %v", err)
		return
	}
	resp, err := cluster.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		t.Fatalf("Cluster client on Get request raised error: %v", err)
		return
	}
	port := "43892"
	t.Logf("--> Spawning server on unreachable port %s", port)
	srv := &http.Server{Addr: fmt.Sprintf(":%s", port), Handler: http.HandlerFunc(NewHandler(t))}
	go func() {
		err := srv.ListenAndServe()
		if err != nil {
			t.Fatalf("Error on spawning server on port %s: %v", port, err)
		}
	}()
	t.Logf("--> Server now running")
	time.Sleep(1*time.Second)

	resp, err = cluster.Do(req)
	defer resp.Body.Close()
	if err != nil {
		t.Fatalf("Cluster client on Get request raised error: %v", err)
		return
	}
}

func TestClusterRespondsErrorIfAllNodesUnavailable(t *testing.T) {
	hosts := []string{"localhost:324786"}
	config := &ClusterConfig{Hosts: hosts, NodeReanimationAfterSeconds: 1}
	cluster, err := NewCluster(config)
	if err != nil {
		t.Fatalf("Unexpected error when create cluster with config `%v`: %v", config, err)
		return
	}
	u := &url.URL{Path: "/"}
	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		t.Fatalf("Unexpected error when create cluster request: %v", err)
		return
	}
	resp, err := cluster.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	if fmt.Sprintf("%v", err) != "No cluster nodes available" {
		t.Fatalf("Missing expected error from request against cluster")
	}
}