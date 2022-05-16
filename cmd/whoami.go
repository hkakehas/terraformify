package cmd

import (
	"fmt"
	"log"
	"os"

	"github.com/fastly/go-fastly/v6/fastly"
	tmfy "github.com/hrmsk66/terraformify/lib"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// whoamiCmd represents the whoami command
var whoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "Get information about the currently authenticated account",
	RunE: func(cmd *cobra.Command, args []string) error {
		filter := tmfy.CreateLogFilter()
		log.SetOutput(filter)
		log.Printf("[INFO] CLI version: %s", version)

		apiKey := viper.GetString("api-key")
		client, err := fastly.NewClient(apiKey)
		if err != nil {
			log.Fatal(err)
		}

		return checkWhoAmI(client)
	},
}

func checkWhoAmI(client *fastly.Client) error {
	user, err := client.GetCurrentUser()
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "CID:\t%s\n", user.CustomerID)
	fmt.Fprintf(os.Stdout, "Name:\t%s\n", user.Name)
	fmt.Fprintf(os.Stdout, "Login:\t%s\n", user.Login)
	return nil
}

func init() {
	rootCmd.AddCommand(whoamiCmd)
}
