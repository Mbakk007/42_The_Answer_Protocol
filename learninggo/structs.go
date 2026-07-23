package main

import "fmt"

type Player struct {
	Name string
	HP int
}


func main(){
	p := Player{Name: "P1", HP: 100}
	fmt.Println(p.Name)
	fmt.Println(p.HP)
	for p.HP > 0 {
		p.HP -= 10
		fmt.Println(p.HP)
	}
}