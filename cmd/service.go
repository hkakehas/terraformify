package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	tmfy "github.com/hrmsk66/terraformify/lib"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// serviceCmd represents the service command
var serviceCmd = &cobra.Command{
	Use:          "service <service-id>",
	Short:        "Generate TF files for an existing Fastly service",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		filter := tmfy.CreateLogFilter()
		log.SetOutput(filter)
		log.Printf("[INFO] CLI version: %s", version)

		workingDir, err := cmd.Flags().GetString("working-dir")
		if err != nil {
			return err
		}
		err = tmfy.CheckDirEmpty(workingDir)
		if err != nil {
			return err
		}

		apiKey := viper.GetString("api-key")
		err = os.Setenv("FASTLY_API_KEY", apiKey)
		if err != nil {
			log.Fatal(err)
		}

		version, err := cmd.Flags().GetInt("version")
		if err != nil {
			return err
		}
		interactive, err := cmd.Flags().GetBool("interactive")
		if err != nil {
			return err
		}
		manageAll, err := cmd.Flags().GetBool("manage-all")
		if err != nil {
			return err
		}
		c := tmfy.Config{
			ID:          args[0],
			Version:     version,
			Directory:   workingDir,
			Interactive: interactive,
			ManageAll:   manageAll,
		}

		return importService(c)
	},
}

func init() {
	rootCmd.AddCommand(serviceCmd)

	// Persistent flags
	serviceCmd.PersistentFlags().IntP("version", "v", 0, "Version of the service to be imported")
	serviceCmd.PersistentFlags().BoolP("manage-all", "m", false, "Manage all associated resources")
}

func importService(c tmfy.Config) error {
	log.Printf("[INFO] Initializing Terraform")
	// Find/Install Terraform binary
	tf, err := tmfy.TerraformInstall(c.Directory)
	if err != nil {
		return err
	}

	// Create provider.tf
	// Create temp*.tf with empty service resource blocks
	log.Printf("[INFO] Creating provider.tf and temp*.tf")
	tempf, err := tmfy.CreateInitTerraformFiles(c)
	defer os.Remove(tempf.Name())
	if err != nil {
		return err
	}

	// Run "terraform init"
	log.Printf(`[INFO] Running "terraform init"`)
	err = tmfy.TerraformInit(tf)
	if err != nil {
		return err
	}

	// Run "terraform version"
	err = tmfy.TerraformVersion(tf)
	if err != nil {
		return err
	}

	// Create VCLServiceResourceProp struct
	serviceProp := tmfy.NewVCLServiceResourceProp(c.ID, "service", c.Version)

	// log.Printf(`[INFO] Running "terraform import %s %s"`, serviceProp.GetRef(), serviceProp.GetIDforTFImport())
	log.Printf(`[INFO] Running "terraform import" on %s`, serviceProp.GetRef())
	err = tmfy.TerraformImport(tf, serviceProp, tempf)
	if err != nil {
		return err
	}

	// Get the config represented in HCL from the "terraform show" output
	log.Print(`[INFO] Running "terraform show" to get the current Terraform state in HCL format`)
	rawHCL, err := tmfy.TerraformShow(tf)

	// Parse HCL and obtain Terraform block props as a list of struct
	// to get the overall picture of the service configuration
	// log.Print("[INFO] Parsing the HCL to get an overall picture of the service configuration")
	log.Print("[INFO] Parsing the HCL")
	tfconf, err := tmfy.LoadTFConf(rawHCL)
	if err != nil {
		return err
	}

	props, err := tfconf.ParseVCLServiceResource(serviceProp, c)
	if err != nil {
		return err
	}

	// Iterate over the list of props and run terraform import for WAF, ACL/dicitonary items, and dynamic snippets
	for _, prop := range props {
		switch r := prop.(type) {
		case *tmfy.WAFResourceProp, *tmfy.ACLResourceProp, *tmfy.DictionaryResourceProp, *tmfy.DynamicSnippetResourceProp:
			// Ask yes/no if in interactive mode
			if c.Interactive {
				yes := tmfy.YesNo(fmt.Sprintf("import %s? ", r.GetRef()))
				if !yes {
					continue
				}
			}

			// log.Printf(`[INFO] Running "terraform import %s %s"`, r.GetRef(), r.GetIDforTFImport())
			log.Printf(`[INFO] Running "terraform import" on %s`, r.GetRef())
			tmfy.TerraformImport(tf, prop, tempf)
			if err != nil {
				return err
			}
		}
	}

	// temp*.tf no longer needed
	if err := tempf.Close(); err != nil {
		return err
	}
	if err := os.Remove(tempf.Name()); err != nil {
		return err
	}

	// Get the config represented in HCL from the "terraform show" output
	log.Print(`[INFO] Running "terraform show" to get the current Terraform state in HCL format`)
	rawHCL, err = tf.ShowPlanFileRaw(context.Background(), "terraform.tfstate")

	// Make changes to the configuration
	// log.Print("[INFO] Parsing the HCL and making corrections removing read-only attrs and replacing embedded VCL/logformat with the file function")
	log.Print("[INFO] Parsing the HCL and making corrections")
	tfconf, err = tmfy.LoadTFConf(rawHCL)
	if err != nil {
		return err
	}

	result, err := tfconf.RewriteResources(serviceProp, c)
	if err != nil {
		return err
	}

	log.Print("[INFO] Writing the configuration to main.tf")
	path := filepath.Join(c.Directory, "main.tf")
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	defer f.Close()
	f.Write(result)

	log.Print(`[INFO] Fixing "activate" attributes in terraform.tfstate`)
	curState, err := tmfy.LoadTFState(c.Directory)
	if err != nil {
		return err
	}
	newState, err := curState.SetActivateAttr()
	if err != nil {
		return err
	}

	if c.ManageAll {
		log.Print(`[INFO] Settting manage_* attributes`)
		newState, err = newState.SetManageAttrs()
		if err != nil {
			return err
		}
	}

	for _, prop := range props {
		switch r := prop.(type) {
		case *tmfy.ACLResourceProp, *tmfy.DictionaryResourceProp, *tmfy.DynamicSnippetResourceProp:
			log.Printf(`[INFO] Setting index keys in terraform.tfstate for %s`, r.GetRef())
			newStateWithTmpl, err := newState.AddIndexKeyQueryTemplate(tmfy.SetIndexKeyQueryTmpl)
			if err != nil {
				return err
			}

			newState, err = newStateWithTmpl.Query(tmfy.IndexKeyQueryParams{
				ResourceType: r.GetType(),
				ResourceName: r.GetNormalizedName(),
				Name:         r.GetName(),
			})
			if err != nil {
				return err
			}
		}
	}

	path = filepath.Join(c.Directory, "terraform.tfstate")
	f, err = os.OpenFile(path, os.O_RDWR|os.O_TRUNC, 0644)
	f.Write(newState.Bytes())
	f.Close()

	log.Print(`[INFO] Running "terraform refresh" to format the state file and check errors`)
	err = tmfy.TerraformRefresh(tf)
	if err != nil {
		return err
	}

	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, tmfy.BoldGreen("Completed!"))
	return nil
}
