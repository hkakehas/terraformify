# terraformify

An experimental CLI for managing existing Fastly resources with Terraform

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

**Note:** The generated main.tf may contain sensitive information such as API keys for logging endpoints; before committing to Git, replace them with variables so that the configuration file does not contain such information.

### Interactive mode

By default, terraformify imports all resources associated with the service, such as ACL entries, dictionary items, WAF..etc. Use the `-i` flag to select which resources to import.

```
terraformify service <service-id> -i
```

### Manage associated resources

By default, the `manage_*` attributes are not set so these resources can be managed externally.

| Resource Name                          | Attribute Name      |
| -------------------------------------- | ------------------- |
| fastly_service_acl_entries             | [manage_entries]()  |
| fastly_service_dictionary_items        | [manage_items]()    |
| fastly_service_dynamic_snippet_content | [manage_snippets]() |

To set attributes to true and manage the resource with Terraform, use the `-m` flag.

```
terraformify service <service-id> -m
```

### Import specific version

By default, either the active version will be imported, or the latest version if no version is active. Alternatively, a specific version of the service can be selected by passing version number to the `-v` flag.

```
terraformify service <service-id> -v 9
```
