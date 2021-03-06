package terraformify

import (
	"bytes"
	"os"
	"testing"
)

const (
	inputFile  = "../testdata/rawHCL.tf"
	goldenFile = "../testdata/golden.tf"
)

func TestRewriteResources(t *testing.T) {
	testCases := []struct {
		serviceID  string
		version    int
		workingDir string

		manageAll bool
	}{
		{
			serviceID:  "6gjZ23Y0k6TApEs5PxzYuT",
			version:    0,
			workingDir: "../testdata",
			manageAll:  false,
		},
	}

	for _, tt := range testCases {
		serviceProp := NewVCLServiceResourceProp(tt.serviceID, "service", tt.version)
		config := Config{
			ID:          tt.serviceID,
			Version:     tt.version,
			Directory:   tt.workingDir,
			Interactive: false,
			ManageAll:   tt.manageAll,
		}

		expected, err := os.ReadFile(goldenFile)
		if err != nil {
			t.Fatal(err)
		}

		b, err := os.ReadFile(inputFile)
		if err != nil {
			t.Fatal(err)
		}

		tfconf, err := LoadTFConf(string(b))
		if err != nil {
			t.Fatal(err)
		}

		result, err := tfconf.RewriteResources(serviceProp, config)
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(expected, result) {
			t.Logf("golden:\n%s\n", expected)
			t.Logf("result:\n%s\n", result)
			t.Error("Result content does not match golden file")
		}

		os.RemoveAll("../testdata/vcl")
		os.RemoveAll("../testdata/content")
		os.RemoveAll("../testdata/logformat")
	}
}
