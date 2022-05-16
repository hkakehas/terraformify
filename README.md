# terraformify

A CLI for migrating existing Fastly resources to Terraform.

![terraformify_demo](https://user-images.githubusercontent.com/30490956/168525136-e23ba260-8aa2-4ff3-a362-963f332b0a94.gif)

## Installation

```
go install github.com/hrmsk66/terraformify@latest
```

## Configuration

terraformify requires read permissions to the target Fastly resource.
Choose one of the following options to give terraformify access to your API token:

- Include the token explicitly on each command you run using the --api-key or -k flags.
- Set a FASTLY_API_KEY environment variable.

## Usage

Run the command in an empty directory

```
mkdir test && cd test
terraformify service <service-id>
```

## How it works

terraformify runs terraform commands behind the scenes and hendle the tedious Terraform migration tasks.

1. Run terraform import on the Fastly service ID given in the command
2. Run terraform show to get an overall picture of the service
3. Run terraform import on resources associated with the service
4. Run terraform show again to get the configuration in HCL format and modify it.
   - Remove read-only attributes
   - Replace VCL and log format embedded in HCL with file function expressions (A Terraform's build-in function)
5. Fetch VCL and log format via Fastly API and save them in the corresponding directories
