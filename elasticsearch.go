package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"sort"

	humanize "github.com/dustin/go-humanize"
)

type NodeStatus struct {
	Name     string
	DiskUsed float64
	HeapUsed int64
	GcTime   int64
}

func (n *NodeStatus) HeapCss() string {
	if n.HeapUsed > 80 {
		return "danger"
	}
	return ""
}

func (n *NodeStatus) GcCss() string {
	if n.GcTime < 20 {
		return "success"
	} else if n.GcTime >= 20 && n.GcTime <= 500 {
		return "warning"
	} else if n.GcTime > 500 {
		return "danger"
	}
	return ""
}

type ByHeap []NodeStatus

func (b ByHeap) Len() int           { return len(b) }
func (b ByHeap) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }
func (b ByHeap) Less(i, j int) bool { return b[i].HeapUsed < b[j].HeapUsed }

type Cluster struct {
	Hostname      string
	Health        ClusterHealthResponse
	history       map[string][]int64
	Current       []NodeStatus
	ShardMovement int
	shardHistory  []int
	Stats         ClusterStatsResponse
}

func (c *Cluster) HumanSize(v string) string {

	switch v {
	case "docs":
		return humanize.Comma(c.Stats.Indices.Docs.Count)
	}
	return ""
}

func (c *Cluster) HealthCss() string {
	switch c.Health.Status {

	case "green":
		return "success"
	case "red":
		return "danger"
	case "yellow":
		return "warning"

	}
	return ""
}

func NewCluster(hostname string) *Cluster {
	return &Cluster{Hostname: hostname, history: map[string][]int64{}}
}

func (c *Cluster) GetStats() {
	resp, err := http.Get(fmt.Sprintf("http://%s/_cluster/stats", c.Hostname))
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	ret := ClusterStatsResponse{}
	jsonErr := json.Unmarshal(body, &ret)
	if jsonErr != nil {
		log.Println(jsonErr)
		return
	}
	c.Stats = ret
}

func (c *Cluster) DiskUsage(stat string) string {
	switch stat {
	case "total":
		inTerabytes := float64(c.Stats.Indices.Store.SizeInBytes) * 0.000000000001
		return fmt.Sprintf("%.02f", inTerabytes)
	}
	return ""
}

func (c *Cluster) GetHealth() {
	resp, err := http.Get(fmt.Sprintf("http://%s/_cluster/health", c.Hostname))
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	ret := ClusterHealthResponse{}
	jsonErr := json.Unmarshal(body, &ret)
	if jsonErr != nil {
		log.Println(jsonErr)
		return
	}
	c.Health = ret
	shardCount := c.Health.InitializingShards + c.Health.RelocatingShards + c.Health.UnassignedShards
	c.shardHistory = append(c.shardHistory, shardCount)
	if len(c.shardHistory) > 60 {
		c.shardHistory = c.shardHistory[1:]
	}
	if len(c.shardHistory) == 60 {
		c.ShardMovement = c.shardHistory[0] - c.shardHistory[59]
	}
}

func (c *Cluster) GetNodeStatus() {
	resp, err := http.Get(fmt.Sprintf("http://%s/_nodes/stats", c.Hostname))
	if err != nil {
		log.Println(err)
		return
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)

	ret := NodeStatsResponse{}
	jsonErr := json.Unmarshal(body, &ret)
	if jsonErr != nil {
		log.Println(jsonErr)
		return
	}

	nodes := []NodeStatus{}

	for _, n := range ret.Nodes {
		name := n.Name
		diskFree := 0.0
		diskTotal := 0.0
		if len(n.FS.Data) > 0 {
			diskFree = float64(n.FS.Data[0].Free)
			diskTotal = float64(n.FS.Data[0].Total)
		}
		diskUsed := Round((1-(diskFree/diskTotal))*100.0, 0.5, 2)
		oldTime := n.JVM.GC.Collectors["old"].TimeInMs
		//youngTime := n.JVM.GC.Collectors["young"].TimeInMs
		gcTime := oldTime //+ youngTime
		if val, ok := c.history[name]; ok {
			c.history[name] = append(val, gcTime)
			if len(c.history[name]) > 2 {
				c.history[name] = c.history[name][1:]
			}
		} else {
			c.history[name] = []int64{gcTime}
		}
		var tmpGcTime int64
		if len(c.history[name]) == 2 {
			tmpGcTime = c.history[name][1] - c.history[name][0]
		}
		tmp := NodeStatus{Name: name, DiskUsed: diskUsed,
			HeapUsed: n.JVM.Mem.HeapUsedPercent,
			GcTime:   tmpGcTime}
		nodes = append(nodes, tmp)
	}
	c.Current = nodes
	sort.Sort(sort.Reverse(ByHeap(c.Current)))
}
