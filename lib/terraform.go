package terraformify

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/hashicorp/go-version"
	"github.com/hashicorp/hc-install/product"
	"github.com/hashicorp/hc-install/releases"
	"github.com/hashicorp/terraform-exec/tfexec"
)

const tfVersion = "1.1.9"
const requiredProvider = `terraform {
  required_providers {
    fastly  = {
      source  = "fastly/fastly"
      version = ">= 2.0.0"
    }
  }
}`

func TerraformInstall(workingDir string) (*tfexec.Terraform, error) {
	execPath, err := exec.LookPath("terraform")
	if err != nil {
		if !errors.Is(err, exec.ErrNotFound) {
			return nil, fmt.Errorf("unknown error when looking for Terraform binaries: %w", err)
		}

		// Install Terraform
		installer := &releases.ExactVersion{
			Product: product.Terraform,
			Version: version.Must(version.NewVersion(tfVersion)),
		}

		execPath, err = installer.Install(context.Background())
		if err != nil {
			return nil, fmt.Errorf("error installing Terraform: %w", err)
		}
	}

	return tfexec.NewTerraform(workingDir, execPath)
}

func CreateInitTerraformFiles(c Config) (*os.File, error) {
	// Create provider.tf
	path := filepath.Join(c.Directory, "provider.tf")
	if err := os.WriteFile(path, []byte(requiredProvider), 0644); err != nil {
		return nil, err
	}

	// Create temp*.tf with empty service resource blocks
	tempf, err := os.CreateTemp(c.Directory, "temp*.tf")
	if err != nil {
		return nil, err
	}

	return tempf, nil
}

func TerraformInit(tf *tfexec.Terraform) error {
	return tf.Init(context.Background(), tfexec.Upgrade(true))
}

func TerraformVersion(tf *tfexec.Terraform) error {
	tfver, providerVers, err := tf.Version(context.Background(), true)
	if err != nil {
		return err
	}

	log.Printf("[INFO] Terraform version: %s on %s_%s", tfver.String(), runtime.GOOS, runtime.GOARCH)
	for k, v := range providerVers {
		log.Printf("[INFO] Provider version: %s %s", k, v.String())
	}
	return nil
}

func TerraformImport(tf *tfexec.Terraform, prop TFBlockProp, f io.Writer) error {
	// Add the empty resource block to the file
	_, err := fmt.Fprintf(f, "resource \"%s\" \"%s\" {}\n", prop.GetType(), prop.GetNormalizedName())
	if err != nil {
		return err
	}

	// Run "terraform import"
	if err := tf.Import(context.Background(), prop.GetRef(), prop.GetIDforTFImport()); err != nil {
		return err
	}

	return nil
}

func TerraformShow(tf *tfexec.Terraform) (string, error) {
	return tf.ShowPlanFileRaw(context.Background(), "terraform.tfstate")
}

func TerraformRefresh(tf *tfexec.Terraform) error {
	return tf.Refresh(context.Background())
}