package main

import (
	"fmt"
	"os"

	"github.com/lucasdbr05/sef-golang/cmd/sef/commands"
	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:   "sef",
		Short: "Secure Fountain (SeF) — LT codes for blockchain storage reduction",
	}

	root.AddCommand(commands.GenerateCmd())
	root.AddCommand(commands.DecodeCmd())

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
