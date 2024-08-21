package jsonschema

import (
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/invopop/jsonschema"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery"
	"github.com/stoewer/go-strcase"
)

func FullyQualifiedName(t reflect.Type) string {
	// Retrieve the package path and type name
	packagePath := t.PkgPath()
	typeName := t.Name()

	// Check if the package path starts with the desired prefix
	//	prefix := "github.com/prometheus/prometheus/discovery"
	if packagePath != "" {
		//	if strings.HasPrefix(packagePath, prefix) {
		// Combine them to form the fully qualified name
		return strings.ReplaceAll(packagePath, "/", ".") + "." + typeName
	}

	return typeName
}
func GenerateJSONSchema(writer io.Writer) error {
	r := new(jsonschema.Reflector)
	r.FieldNameTag = "yaml"
	r.Namer = FullyQualifiedName
	r.DoNotReference = true
	r.KeyNamer = strcase.SnakeCase
	r.RequiredFromJSONSchemaTags = true

	// Create a composite config that includes dynamically registered SD configs
	mainConfig := &config.Config{}
	configType := reflect.TypeOf(mainConfig).Elem()

	// Get the scrape_configs and alerting_configs.AlertmanagerConfigs fields
	scrapeConfigsField, _ := configType.FieldByName("ScrapeConfigs")
	alertingConfigField, ok := configType.FieldByName("AlertingConfig")
	if !ok {
		return fmt.Errorf("could not find AlertingConfig field in config type")
	}

	alertmanagerConfigsField, ok := alertingConfigField.Type.FieldByName("AlertmanagerConfigs")
	if !ok {
		return fmt.Errorf("could not find AlertmanagerConfigs field in AlertingConfig")
	}

	// Handle pointer dereferencing for ScrapeConfig and AlertmanagerConfig types
	scrapeConfigType := scrapeConfigsField.Type.Elem()
	if scrapeConfigType.Kind() == reflect.Ptr {
		scrapeConfigType = scrapeConfigType.Elem()
	}

	alertmanagerConfigType := alertmanagerConfigsField.Type.Elem()
	if alertmanagerConfigType.Kind() == reflect.Ptr {
		alertmanagerConfigType = alertmanagerConfigType.Elem()
	}

	// Prepare to create the new composite struct for ScrapeConfig
	scrapeConfigFields := []reflect.StructField{}
	for i := 0; i < scrapeConfigType.NumField(); i++ {
		scrapeConfigFields = append(scrapeConfigFields, scrapeConfigType.Field(i))
	}

	// Prepare to create the new composite struct for AlertmanagerConfig
	alertmanagerConfigFields := []reflect.StructField{}
	for i := 0; i < alertmanagerConfigType.NumField(); i++ {
		alertmanagerConfigFields = append(alertmanagerConfigFields, alertmanagerConfigType.Field(i))
	}

	// Iterate over the registered SD configs and add them to the appropriate fields
	for typeName, registeredType := range discovery.GetRegisteredConfigs() {
		if typeName == "" {
			continue
		}
		field := reflect.StructField{
			Name: typeName,
			Type: reflect.SliceOf(registeredType),
			Tag:  reflect.StructTag(`yaml:"` + discovery.GetFieldName(typeName) + `,omitempty"`),
		}

		// Add the field to both ScrapeConfig and AlertmanagerConfig
		scrapeConfigFields = append(scrapeConfigFields, field)
		alertmanagerConfigFields = append(alertmanagerConfigFields, field)
	}

	// Create the new composite struct types
	newScrapeConfigType := reflect.StructOf(scrapeConfigFields)
	newAlertmanagerConfigType := reflect.StructOf(alertmanagerConfigFields)

	// Update the main fields in the composite struct
	fields := []reflect.StructField{
		{
			Name: scrapeConfigsField.Name,
			Type: reflect.SliceOf(newScrapeConfigType),
			Tag:  scrapeConfigsField.Tag,
		},
		{
			Name: alertingConfigField.Name,
			Type: reflect.StructOf([]reflect.StructField{
				{
					Name: alertmanagerConfigsField.Name,
					Type: reflect.SliceOf(newAlertmanagerConfigType),
					Tag:  alertmanagerConfigsField.Tag,
				},
			}),
			Tag: alertingConfigField.Tag,
		},
	}

	// Create the new composite struct type and generate the JSON schema
	newConfigType := reflect.StructOf(fields)
	newConfig := reflect.New(newConfigType).Interface()
	schema := r.Reflect(newConfig)

	prettyJson, err := json.MarshalIndent(schema, "", "    ")
	if err != nil {
		return err
	}

	_, err = writer.Write(prettyJson)
	return err
}
