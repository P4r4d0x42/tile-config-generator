package metadata

import (
	"archive/zip"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	pivnetapi "github.com/pivotal-cf/go-pivnet"
	"github.com/pivotal-cf/go-pivnet/logshim"
	"github.com/pkg/errors"
	"howett.net/ranger"
)

func NewPivnetProvider(token, slug, version, glob string) Provider {

	logWriter := os.Stderr
	logger := log.New(logWriter, "", log.LstdFlags)
	config := pivnetapi.ClientConfig{
		Host:      pivnetapi.DefaultHost,
		Token:     token,
		UserAgent: "tile-config-generator",
	}
	ls := logshim.NewLogShim(logger, logger, false)
	client := pivnetapi.NewClient(config, ls)
	pivnetAuthClient := AuthenticatedPivnetClient{ClientConfig: config, HTTPClient: client.HTTP}
	return &PivnetProvider{
		client:           client,
		pivnetAuthClient: pivnetAuthClient,
		progressWriter:   os.Stderr,
		logger:           ls,
		slug:             slug,
		version:          version,
		glob:             glob,
	}
}

type PivnetProvider struct {
	client           pivnetapi.Client
	pivnetAuthClient AuthenticatedPivnetClient
	progressWriter   io.Writer
	logger           *logshim.LogShim
	slug             string
	version          string
	glob             string
}

func (p *PivnetProvider) MetadataBytes() ([]byte, error) {

	releases, err := p.client.Releases.List(p.slug)
	if err != nil {
		return nil, err
	}

	for _, release := range releases {
		if release.Version == p.version {
			productFiles, err := p.client.ProductFiles.ListForRelease(p.slug, release.ID)
			if err != nil {
				return nil, err
			}
			err = p.client.EULA.Accept(p.slug, release.ID)
			if err != nil {
				return nil, err
			}
			return p.downloadFiles(productFiles, release.ID)

		}
	}

	return nil, nil
}

func (p *PivnetProvider) downloadFiles(
	productFiles []pivnetapi.ProductFile,
	releaseID int,
) ([]byte, error) {

	filtered := productFiles

	var err error
	filtered, err = productFileKeysByGlobs(productFiles, p.glob)
	if err != nil {
		return nil, err
	}

	if len(filtered) == 0 {
		return nil, fmt.Errorf("No file matched for slug %s, releaseID %d and glob %s", p.slug, releaseID, p.glob)
	}
	if len(filtered) > 1 {
		return nil, fmt.Errorf("More than one file matched for slug %s, releaseID %d and glob %s", p.slug, releaseID, p.glob)
	}

	pf := filtered[0]
	downloadLink, err := pf.DownloadLink()
	if err != nil {
		return nil, err
	}

	url, _ := url.Parse(downloadLink)

	r := &ranger.HTTPRanger{URL: url, Client: p.pivnetAuthClient}
	reader, err := ranger.NewReader(r)
	if err != nil {
		return nil, errors.Wrap(err, "error with New Reader")
	}
	length, err := reader.Length()
	if err != nil {
		return nil, errors.Wrap(err, "error with reader length")
	}
	zipreader, err := zip.NewReader(reader, length)
	if err != nil {
		return nil, errors.Wrap(err, "error with New Zip Reader")
	}

	var matcher = regexp.MustCompile(`metadata/(.*)yml`)
	for _, file := range zipreader.File {
		if matcher.MatchString(file.Name) {
			data := make([]byte, file.UncompressedSize64)
			rc, err := file.Open()
			if err != nil {
				return nil, err
			}
			io.ReadFull(rc, data)
			rc.Close()
			return data, nil
		}

	}
	return nil, fmt.Errorf("No metadata found")
}

func productFileKeysByGlobs(
	productFiles []pivnetapi.ProductFile,
	pattern string,
) ([]pivnetapi.ProductFile, error) {

	filtered := []pivnetapi.ProductFile{}

	for _, p := range productFiles {
		parts := strings.Split(p.AWSObjectKey, "/")
		fileName := parts[len(parts)-1]

		matched, err := filepath.Match(pattern, fileName)
		if err != nil {
			return nil, err
		}

		if matched {
			filtered = append(filtered, p)
		}
	}

	if len(filtered) == 0 && pattern != "" {
		return nil, fmt.Errorf("no match for pattern: '%s'", pattern)
	}

	return filtered, nil
}

type AuthenticatedPivnetClient struct {
	ClientConfig pivnetapi.ClientConfig
	HTTPClient   *http.Client
}

func (a AuthenticatedPivnetClient) Do(req *http.Request) (*http.Response, error) {
	if a.ClientConfig.UsingUAAToken {
		req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", a.ClientConfig.Token))
	} else {
		req.Header.Add("Authorization", fmt.Sprintf("Token %s", a.ClientConfig.Token))
	}
	req.Header.Add("User-Agent", a.ClientConfig.UserAgent)
	resp, err := a.HTTPClient.Do(req)
	if err != nil {
		log.Fatal(err.Error())
	}
	resp.Header.Add("Content-Type", "application/multipart")
	return resp, err
}
func (a AuthenticatedPivnetClient) Get(url string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	return a.Do(req)
}
func (a AuthenticatedPivnetClient) Head(url string) (*http.Response, error) {
	req, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return nil, err
	}
	return a.Do(req)
}
