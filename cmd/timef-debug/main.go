package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/oliverschoning/timef/internal/session"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: timef-debug <path>")
		os.Exit(1)
	}
	c, err := session.NewClient()
	if err != nil {
		fmt.Println("err:", err)
		os.Exit(1)
	}
	data, err := c.Do("GET", os.Args[1], nil)
	if err != nil {
		fmt.Println("err:", err)
		os.Exit(1)
	}
	var v interface{}
	if json.Unmarshal(data, &v) == nil {
		out, _ := json.MarshalIndent(v, "", "  ")
		fmt.Println(string(out))
		return
	}
	fmt.Println(string(data))
}
