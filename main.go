package main

import (
	"bufio"
	"log"
	"math"
	"net/http"
	"os"
	"text/template"
	"time"
)

var (
	current = []NodeStatus{}
)

func Round(val float64, roundOn float64, places int) (newVal float64) {
	var round float64
	pow := math.Pow(10, float64(places))
	digit := pow * val
	_, div := math.Modf(digit)
	if div >= roundOn {
		round = math.Ceil(digit)
	} else {
		round = math.Floor(digit)
	}
	newVal = round / pow
	return
}

func Render(w http.ResponseWriter, name string, extra interface{}) {
	w.Header().Set("Content-Type", "text/html")
	tpl := template.New(name)
	tpl, err := tpl.ParseFiles("templates/" + name + ".html")
	if err != nil {
		panic(err)
	}
	tpl.ExecuteTemplate(w, name, extra)
}

func main() {
	clusters := []*Cluster{}

	file, err := os.Open("hosts.cfg")
	if err != nil {
		log.Fatal(err)
	}

	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		clusters = append(clusters, NewCluster(scanner.Text()))
	}

	for _, c := range clusters {
		go func(c *Cluster) {
			for {
				c.GetStats()
				c.GetHealth()
				c.GetNodeStatus()
				time.Sleep(1 * time.Second)
			}
		}(c)
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		Render(w, "index", struct{ Clusters []*Cluster }{clusters})
	})

	log.Fatal(http.ListenAndServe(":8080", nil))

}
