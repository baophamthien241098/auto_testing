package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

type ModelList struct {
	Models []struct {
		Name        string `json:"name"`
		DisplayName string `json:"displayName"`
	} `json:"models"`
}

func main() {
	apiKey := os.Args[1]
	url := "https://generativelanguage.googleapis.com/v1beta/models?key=" + apiKey

	resp, err := http.Get(url)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var list ModelList
	json.Unmarshal(body, &list)

	fmt.Println("AVAILABLE MODELS:")
	for _, m := range list.Models {
		fmt.Printf("- %s (ID: %s)\n", m.DisplayName, m.Name)
	}
}
