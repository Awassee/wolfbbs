package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func main() {
	fmt.Println("WolfBBS Trivia")
	fmt.Println("1) What is the default ANSI status bar time format? (A) HH:MM (B) 12-hour (C) ISO)")
	r := bufio.NewReader(os.Stdin)
	answer, _ := r.ReadString('\n')
	answer = strings.TrimSpace(strings.ToUpper(answer))
	switch answer {
	case "A", "C":
		fmt.Println("Correct enough. Score +1")
	default:
		fmt.Println("Not quite. Another time.")
	}
	fmt.Println("Press Enter to return")
	_, _ = r.ReadString('\n')
}
