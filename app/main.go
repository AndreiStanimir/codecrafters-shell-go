package main

import (
	"bufio"
	"fmt"
	"os"
)

// Ensures gofmt doesn't remove the "fmt" and "os" imports in stage 1 (feel free to remove this!)
var (
	_ = fmt.Fprint
	_ = os.Stdout
)

func main() {
	// TODO: Uncomment the code below to pass the first stage
	fmt.Fprint(os.Stdout, "$ ")
	command, err := bufio.NewReader(os.Stdin).ReadString('\n')

	fmt.Fprintf("%v: command not found", command)
}
