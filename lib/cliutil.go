package terraformify

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/fastly/go-fastly/v6/fastly"
	"github.com/fatih/color"
	"github.com/hashicorp/logutils"
)

type Config struct {
	ID          string
	Version     int
	Directory   string
	Interactive bool
	Client      *fastly.Client
}

var Bold = color.New(color.Bold).SprintFunc()
var BoldGreen = color.New(color.Bold, color.FgGreen).SprintFunc()

func CreateLogFilter() io.Writer {
	minLevel := os.Getenv("TMFY_LOG")
	if minLevel == "" {
		minLevel = "INFO"
	}
	filter := &logutils.LevelFilter{
		Levels:   []logutils.LogLevel{"DEBUG", "INFO", "WARN", "ERROR"},
		MinLevel: logutils.LogLevel(minLevel),
		Writer:   os.Stderr,
	}
	return filter
}

func CheckDirEmpty(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		log.Fatal(err)
	}
	if !info.IsDir() {
		log.Fatal(fmt.Errorf("%s is not a directory", path))
	}

	d, err := os.Open(path)
	if err != nil {
		return err
	}
	defer d.Close()

	_, err = d.Readdir(1)
	if err == io.EOF {
		return nil
	}

	return errors.New("Working directory is not empty")
}

func YesNo(message string) (bool, error) {
	var s string

	fmt.Printf("%s (y/N): ", message)
	_, err := fmt.Scan(&s)
	if err != nil {
		return false, err
	}

	s = strings.TrimSpace(s)
	s = strings.ToLower(s)

	if s == "y" || s == "yes" {
		return true, nil
	}
	return false, nil
}
