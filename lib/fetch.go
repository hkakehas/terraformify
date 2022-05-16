package terraformify

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fastly/go-fastly/v6/fastly"
)

func FetchAssetsViaFastlyAPI(props []TFBlockProp, c Config) error {
	for _, prop := range props {
		switch p := prop.(type) {
		case *SnippetBlockProp, *VCLBlockProp, *DynamicSnippetResourceProp:
			path := filepath.Join(c.Directory, "vcl")
			if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
				err := os.Mkdir(path, 0755)
				if err != nil {
					return err
				}
			}
			switch p := prop.(type) {
			case *SnippetBlockProp:
				if err := fetchVCLSnippet(p, c); err != nil {
					return err
				}
			case *VCLBlockProp:
				if err := fetchCustomVCL(p, c); err != nil {
					return err
				}
			case *DynamicSnippetResourceProp:
				if err := fetchDynamicSnippet(p, c); err != nil {
					return err
				}
			}
		case *LoggingBlockProp:
			path := filepath.Join(c.Directory, "logformat")
			if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
				err := os.Mkdir(path, 0755)
				if err != nil {
					return err
				}
			}
			if err := fetchLoggingFormat(p, c); err != nil {
				return err
			}
		}
	}
	return nil
}

func fetchCustomVCL(v *VCLBlockProp, c Config) error {
	vcl, err := c.Client.GetVCL(&fastly.GetVCLInput{
		ServiceID:      v.GetID(),
		ServiceVersion: v.GetVersion(),
		Name:           v.GetName(),
	})
	if err != nil {
		return err
	}

	prefix := ""
	ext := ".vcl"
	path := filepath.Join(c.Directory, "vcl", prefix+v.GetNormalizedName()+ext)
	return os.WriteFile(path, []byte(vcl.Content), 0644)
}

func fetchVCLSnippet(s *SnippetBlockProp, c Config) error {
	vcl, err := c.Client.GetSnippet(&fastly.GetSnippetInput{
		ServiceID:      s.GetID(),
		ServiceVersion: s.GetVersion(),
		Name:           s.GetName(),
	})
	if err != nil {
		return err
	}

	prefix := "snippet_"
	ext := ".vcl"
	path := filepath.Join(c.Directory, "vcl", prefix+s.GetNormalizedName()+ext)
	return os.WriteFile(path, []byte(vcl.Content), 0644)
}

func fetchDynamicSnippet(d *DynamicSnippetResourceProp, c Config) error {
	vcl, err := c.Client.GetDynamicSnippet(&fastly.GetDynamicSnippetInput{
		ServiceID: d.VCLServiceResourceProp.GetID(),
		ID:        d.GetID(),
	})
	if err != nil {
		return err
	}

	prefix := "dsnippet_"
	ext := ".vcl"
	path := filepath.Join(c.Directory, "vcl", prefix+d.GetNormalizedName()+ext)
	return os.WriteFile(path, []byte(vcl.Content), 0644)
}

func fetchLoggingFormat(l *LoggingBlockProp, c Config) error {
	var format string

	switch l.GetEndpointType() {
	case "logging_bigquery":
		log, err := c.Client.GetBigQuery(&fastly.GetBigQueryInput{
			ServiceID:      l.GetID(),
			ServiceVersion: l.GetVersion(),
			Name:           l.GetName(),
		})
		if err != nil {
			return err
		}
		format = log.Format
	case "logging_blobstorage":
		log, err := c.Client.GetBlobStorage(&fastly.GetBlobStorageInput{
			ServiceID:      l.GetID(),
			ServiceVersion: l.GetVersion(),
			Name:           l.GetName(),
		})
		if err != nil {
			return err
		}
		format = log.Format
	case "logging_cloudfiles":
		log, err := c.Client.GetCloudfiles(&fastly.GetCloudfilesInput{
			ServiceID:      l.GetID(),
			ServiceVersion: l.GetVersion(),
			Name:           l.GetName(),
		})
		if err != nil {
			return err
		}
		format = log.Format
	case "logging_datadog":
		log, err := c.Client.GetDatadog(&fastly.GetDatadogInput{
			ServiceID:      l.GetID(),
			ServiceVersion: l.GetVersion(),
			Name:           l.GetName(),
		})
		if err != nil {
			return err
		}
		format = log.Format
	case "logging_digitalocean":
		log, err := c.Client.GetDigitalOcean(&fastly.GetDigitalOceanInput{
			ServiceID:      l.GetID(),
			ServiceVersion: l.GetVersion(),
			Name:           l.GetName(),
		})
		if err != nil {
			return err
		}
		format = log.Format
	case "logging_elasticsearch":
		log, err := c.Client.GetElasticsearch(&fastly.GetElasticsearchInput{
			ServiceID:      l.GetID(),
			ServiceVersion: l.GetVersion(),
			Name:           l.GetName(),
		})
		if err != nil {
			return err
		}
		format = log.Format
	case "logging_ftp":
		log, err := c.Client.GetFTP(&fastly.GetFTPInput{
			ServiceID:      l.GetID(),
			ServiceVersion: l.GetVersion(),
			Name:           l.GetName(),
		})
		if err != nil {
			return err
		}
		format = log.Format
	case "logging_gcs":
		log, err := c.Client.GetGCS(&fastly.GetGCSInput{
			ServiceID:      l.GetID(),
			ServiceVersion: l.GetVersion(),
			Name:           l.GetName(),
		})
		if err != nil {
			return err
		}
		format = log.Format
	case "logging_googlepubsub":
		log, err := c.Client.GetPubsub(&fastly.GetPubsubInput{
			ServiceID:      l.GetID(),
			ServiceVersion: l.GetVersion(),
			Name:           l.GetName(),
		})
		if err != nil {
			return err
		}
		format = log.Format
	case "logging_heroku":
		log, err := c.Client.GetHeroku(&fastly.GetHerokuInput{
			ServiceID:      l.GetID(),
			ServiceVersion: l.GetVersion(),
			Name:           l.GetName(),
		})
		if err != nil {
			return err
		}
		format = log.Format
	case "logging_honeycomb":
		log, err := c.Client.GetHoneycomb(&fastly.GetHoneycombInput{
			ServiceID:      l.GetID(),
			ServiceVersion: l.GetVersion(),
			Name:           l.GetName(),
		})
		if err != nil {
			return err
		}
		format = log.Format
	case "logging_https":
		log, err := c.Client.GetHTTPS(&fastly.GetHTTPSInput{
			ServiceID:      l.GetID(),
			ServiceVersion: l.GetVersion(),
			Name:           l.GetName(),
		})
		if err != nil {
			return err
		}
		format = log.Format
	case "logging_kafka":
		log, err := c.Client.GetKafka(&fastly.GetKafkaInput{
			ServiceID:      l.GetID(),
			ServiceVersion: l.GetVersion(),
			Name:           l.GetName(),
		})
		if err != nil {
			return err
		}
		format = log.Format
	case "logging_kinesis":
		log, err := c.Client.GetKinesis(&fastly.GetKinesisInput{
			ServiceID:      l.GetID(),
			ServiceVersion: l.GetVersion(),
			Name:           l.GetName(),
		})
		if err != nil {
			return err
		}
		format = log.Format
	case "logging_logentries":
		log, err := c.Client.GetLogentries(&fastly.GetLogentriesInput{
			ServiceID:      l.GetID(),
			ServiceVersion: l.GetVersion(),
			Name:           l.GetName(),
		})
		if err != nil {
			return err
		}
		format = log.Format
	case "logging_loggly":
		log, err := c.Client.GetLoggly(&fastly.GetLogglyInput{
			ServiceID:      l.GetID(),
			ServiceVersion: l.GetVersion(),
			Name:           l.GetName(),
		})
		if err != nil {
			return err
		}
		format = log.Format
	case "logging_logshuttle":
		log, err := c.Client.GetLogshuttle(&fastly.GetLogshuttleInput{
			ServiceID:      l.GetID(),
			ServiceVersion: l.GetVersion(),
			Name:           l.GetName(),
		})
		if err != nil {
			return err
		}
		format = log.Format
	case "logging_newrelic":
		log, err := c.Client.GetNewRelic(&fastly.GetNewRelicInput{
			ServiceID:      l.GetID(),
			ServiceVersion: l.GetVersion(),
			Name:           l.GetName(),
		})
		if err != nil {
			return err
		}
		format = log.Format
	case "logging_openstack":
		log, err := c.Client.GetOpenstack(&fastly.GetOpenstackInput{
			ServiceID:      l.GetID(),
			ServiceVersion: l.GetVersion(),
			Name:           l.GetName(),
		})
		if err != nil {
			return err
		}
		format = log.Format
	case "logging_papertrail":
		log, err := c.Client.GetPapertrail(&fastly.GetPapertrailInput{
			ServiceID:      l.GetID(),
			ServiceVersion: l.GetVersion(),
			Name:           l.GetName(),
		})
		if err != nil {
			return err
		}
		format = log.Format
	case "logging_s3":
		log, err := c.Client.GetS3(&fastly.GetS3Input{
			ServiceID:      l.GetID(),
			ServiceVersion: l.GetVersion(),
			Name:           l.GetName(),
		})
		if err != nil {
			return err
		}
		format = log.Format
	case "logging_scalyr":
		log, err := c.Client.GetScalyr(&fastly.GetScalyrInput{
			ServiceID:      l.GetID(),
			ServiceVersion: l.GetVersion(),
			Name:           l.GetName(),
		})
		if err != nil {
			return err
		}
		format = log.Format
	case "logging_sftp":
		log, err := c.Client.GetSFTP(&fastly.GetSFTPInput{
			ServiceID:      l.GetID(),
			ServiceVersion: l.GetVersion(),
			Name:           l.GetName(),
		})
		if err != nil {
			return err
		}
		format = log.Format
	case "logging_splunk":
		log, err := c.Client.GetSplunk(&fastly.GetSplunkInput{
			ServiceID:      l.GetID(),
			ServiceVersion: l.GetVersion(),
			Name:           l.GetName(),
		})
		if err != nil {
			return err
		}
		format = log.Format
	case "logging_sumologic":
		log, err := c.Client.GetSumologic(&fastly.GetSumologicInput{
			ServiceID:      l.GetID(),
			ServiceVersion: l.GetVersion(),
			Name:           l.GetName(),
		})
		if err != nil {
			return err
		}
		format = log.Format
	case "logging_syslog":
		log, err := c.Client.GetSyslog(&fastly.GetSyslogInput{
			ServiceID:      l.GetID(),
			ServiceVersion: l.GetVersion(),
			Name:           l.GetName(),
		})
		if err != nil {
			return err
		}
		format = log.Format
	default:
		return fmt.Errorf("%w: %s", ErrInvalidLogEndpoint, l.EndpointType)
	}

	ext := ".txt"
	if l.IsJSON {
		ext = ".json"
	}
	path := filepath.Join(c.Directory, "logformat", l.GetNormalizedName()+ext)
	return os.WriteFile(path, []byte(format), 0644)
}
