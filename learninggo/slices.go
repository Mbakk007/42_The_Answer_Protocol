package main

import "fmt"

func removeat(s []string, index int) []string {
	return append(s[:index], s[index + 1:]...)
}


func main(){
	var items[]string
	items = []string{"sword", "shield"}

	var inventory[]string
	inventory = append(inventory, items...)
	inventory = append(inventory, "food")
	inventory = append(inventory, "potion")
	inventory = append(inventory, "water")
	for _, item := range inventory {
		fmt.Println(item)
	}
	fmt.Println("___________________________________________")
	inventory = removeat(inventory, 1)
	fmt.Println("___________________________________________")
	for _, item := range inventory {
		fmt.Println(item)
	}
}