package generator

import (
	"archive/zip"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"regexp"

	"gopkg.in/yaml.v2"
)

type Executor struct {
	PathToPivotalFile          string
	BaseDirectory              string
	DoNotIncludeProductVersion bool
}

func NewExecutor(filePath, baseDirectory string, doNotIncludeProductVersion bool) *Executor {
	return &Executor{
		PathToPivotalFile:          filePath,
		BaseDirectory:              baseDirectory,
		DoNotIncludeProductVersion: doNotIncludeProductVersion,
	}
}

func (e *Executor) Generate() error {
	metadataBytes, err := e.extractMetadataBytes()
	if err != nil {
		return err
	}
	metadata, err := NewMetadata(metadataBytes)
	if err != nil {
		return err
	}
	providesVersion := metadata.ProvidesVersions[0]
	targetDirectory := e.BaseDirectory
	if !e.DoNotIncludeProductVersion {
		targetDirectory = path.Join(e.BaseDirectory, providesVersion.Name, providesVersion.Version)
	}
	if err = e.createDirectory(targetDirectory); err != nil {
		return err
	}

	featuresDirectory := path.Join(targetDirectory, "features")
	if err = e.createDirectory(featuresDirectory); err != nil {
		return err
	}

	optionalDirectory := path.Join(targetDirectory, "optional")
	if err = e.createDirectory(optionalDirectory); err != nil {
		return err
	}

	networkDirectory := path.Join(targetDirectory, "network")
	if err = e.createDirectory(networkDirectory); err != nil {
		return err
	}

	resourceDirectory := path.Join(targetDirectory, "resource")
	if err = e.createDirectory(resourceDirectory); err != nil {
		return err
	}

	template, err := e.CreateTemplate(metadata)
	if err != nil {
		return err
	}

	template.ProductName = providesVersion.Name
	template.ProductVersion = metadata.Version

	if err = e.writeYamlFile(path.Join(targetDirectory, "product.yml"), template); err != nil {
		return err
	}

	networkOpsFiles, err := CreateNetworkOpsFiles(metadata)
	if err != nil {
		return err
	}

	if len(networkOpsFiles) > 0 {
		for name, contents := range networkOpsFiles {
			if err = e.writeYamlFile(path.Join(networkDirectory, fmt.Sprintf("%s.yml", name)), contents); err != nil {
				return err
			}
		}
	}

	resourceVars := CreateResourceVars(metadata)

	if len(resourceVars) > 0 {
		if err = e.writeYamlFile(path.Join(targetDirectory, "resource-vars.yml"), resourceVars); err != nil {
			return err
		}
	}

	resourceOpsFiles, err := CreateResourceOpsFiles(metadata)
	if err != nil {
		return err
	}

	if len(resourceOpsFiles) > 0 {
		for name, contents := range resourceOpsFiles {
			if err = e.writeYamlFile(path.Join(resourceDirectory, fmt.Sprintf("%s.yml", name)), contents); err != nil {
				return err
			}
		}
	}

	productPropertyVars, err := CreateProductPropertiesVars(metadata)
	if err != nil {
		return err
	}

	if len(productPropertyVars) > 0 {
		if err = e.writeYamlFile(path.Join(targetDirectory, "product-default-vars.yml"), productPropertyVars); err != nil {
			return err
		}
	}

	productPropertyOpsFiles, err := CreateProductPropertiesFeaturesOpsFiles(metadata)
	if err != nil {
		return err
	}

	if len(productPropertyOpsFiles) > 0 {
		for name, contents := range productPropertyOpsFiles {
			if err = e.writeYamlFile(path.Join(featuresDirectory, fmt.Sprintf("%s.yml", name)), contents); err != nil {
				return err
			}
		}
	}

	productPropertyOptionalOpsFiles, err := CreateProductPropertiesOptionalOpsFiles(metadata)
	if err != nil {
		return err
	}

	if len(productPropertyOptionalOpsFiles) > 0 {
		for name, contents := range productPropertyOptionalOpsFiles {
			if err = e.writeYamlFile(path.Join(optionalDirectory, fmt.Sprintf("%s.yml", name)), contents); err != nil {
				return err
			}
		}
	}

	return nil
}

func (e *Executor) CreateTemplate(metadata *Metadata) (*Template, error) {
	template := &Template{}
	template.NetworkProperties = CreateNetworkProperties(metadata)
	template.ResourceConfig = CreateResourceConfig(metadata)
	productProperties, err := CreateProductProperties(metadata)
	if err != nil {
		return nil, err
	}
	template.ProductProperties = productProperties
	return template, nil
}

func (e *Executor) createDirectory(path string) error {
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		if err := os.MkdirAll(path, 0755); err != nil {
			return fmt.Errorf("cannot create directory %s: %v", path, err)
		}
	}

	return nil
}

func (e *Executor) extractMetadataBytes() ([]byte, error) {
	zipReader, err := zip.OpenReader(e.PathToPivotalFile)
	if err != nil {
		return nil, err
	}

	defer zipReader.Close()

	for _, file := range zipReader.File {
		metadataRegexp := regexp.MustCompile("metadata/.*\\.yml")
		matched := metadataRegexp.MatchString(file.Name)

		if matched {
			metadataFile, err := file.Open()
			contents, err := ioutil.ReadAll(metadataFile)
			if err != nil {
				return nil, err
			}
			return contents, nil
		}
	}
	return nil, errors.New("no metadata file was found in provided .pivotal")
}

func (e *Executor) writeYamlFile(targetFile string, dataType interface{}) error {
	data, err := yaml.Marshal(dataType)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(targetFile, data, 0755)
}
