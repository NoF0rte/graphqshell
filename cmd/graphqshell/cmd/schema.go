package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// schemaCmd represents the schema command
var schemaCmd = &cobra.Command{
	Use:   "schema",
	Short: "A brief description of your command",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("schema called")
	},
}

func init() {
	rootCmd.AddCommand(schemaCmd)
}
