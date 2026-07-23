package main

import ("fmt"
		"os"
		"strconv"
) 

func add(a float64, b float64) float64{
	return a + b
}

func subtract(a float64, b float64) float64{
	return a - b
}

func multiply(a, b float64) float64{
	return a * b
}

func divide(a, b float64) (float64, error){
	if b == 0 {
		return 0, fmt.Errorf("Cannot divide by 0")
	}
	return a / b, nil
}

func main() {
	if len(os.Args) != 4 {
		fmt.Println("Usage: <op> <a> <b>")
		os.Exit(1)
	}
	op := os.Args[1]
	a, err := strconv.ParseFloat(os.Args[2], 64)
	if err != nil {
		fmt.Println("invalid num", os.Args[2])
		os.Exit(1)
	}
	b, err := strconv.ParseFloat(os.Args[3], 64)
	if err != nil {
		fmt.Println("invalid num", os.Args[3])
		os.Exit(1)
	}
	var result float64
	switch op {
	case "add":
		result = add(a, b)
	case "sub":
		result = subtract(a, b)
	case "mul":
		result = multiply(a, b)
	case "div":
		result, err = divide(a, b)
		if err != nil {
			fmt.Println("error", err)
			os.Exit(1)
		}
	default:
		fmt.Println("unknown op:", op)
		os.Exit(1)
	}
	fmt.Println("result:", result)
}