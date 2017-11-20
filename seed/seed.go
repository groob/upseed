// Package seed provides utilities for downloading and parsing Apple SUS Catalogs.
package seed

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/groob/plist"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
)

const CustomerSeed = "https://swscan.apple.com/content/catalogs/others/index-10.13customerseed-10.13-10.12-10.11-10.10-10.9-mountainlion-lion-snowleopard-leopard.merged-1.sucatalog"

type Client struct {
	httpClient           *http.Client
	logger               log.Logger
	catalogURL           string
	localCatalogFilePath string
}

type ClientOption func(*Client)

func WithCatalogURL(url string) ClientOption {
	return func(c *Client) {
		c.catalogURL = url
	}
}

func WithCatalogFromFile(path string) ClientOption {
	return func(c *Client) {
		c.localCatalogFilePath = path
	}
}

func WithHTTPClient(client *http.Client) ClientOption {
	return func(c *Client) {
		c.httpClient = client
	}
}

func New(opts ...ClientOption) *Client {
	client := &Client{
		httpClient: http.DefaultClient,
		logger:     log.NewNopLogger(),
		catalogURL: CustomerSeed,
	}

	for _, optFunc := range opts {
		optFunc(client)
	}
	return client
}

func parseLocalCatalog(path string) (*Catalog, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.Wrapf(err, "opening catalog from path %s", path)
	}
	defer file.Close()
	return parseCatalog(file)
}

func (c *Client) Catalog(ctx context.Context) (*Catalog, error) {
	if c.localCatalogFilePath != "" {
		return parseLocalCatalog(c.localCatalogFilePath)
	}
	req, err := http.NewRequest("GET", c.catalogURL, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "create request to get catalog %s", c.catalogURL)
	}
	req = req.WithContext(ctx)
	req.Header.Set("User-Agent", "curl/7.37.0")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return parseCatalog(resp.Body)
}

type FindOSInstallersOption func(*findOSInstallersOpts)

func FindOSInstallersUsingCatalog(c *Catalog) FindOSInstallersOption {
	return func(conf *findOSInstallersOpts) {
		conf.catalog = c
	}
}

type findOSInstallersOpts struct {
	catalog *Catalog
}

func (c *Client) FindOSInstallers(ctx context.Context, opts ...FindOSInstallersOption) ([]InstallerInfo, error) {
	var conf findOSInstallersOpts
	for _, optFunc := range opts {
		optFunc(&conf)
	}

	catalog := conf.catalog
	if catalog == nil {
		ct, err := c.Catalog(ctx)
		if err != nil {
			return nil, err
		}
		catalog = ct
	}

	productKeys := findmacOSInstallers(catalog)
	known := make(map[string]struct{})
	var infos []InstallerInfo
	eg, gctx := errgroup.WithContext(ctx)
	for _, k := range productKeys {
		k := k // https://golang.org/doc/faq#closures_and_goroutines
		eg.Go(func() error {
			distURL, err := catalog.EnglishDistURLForProduct(k.productKey)
			if err != nil {
				return err
			}
			req, err := http.NewRequest("GET", distURL, nil)
			if err != nil {
				return err
			}
			req = req.WithContext(gctx)
			resp, err := c.httpClient.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			info, err := parseDist(resp.Body)
			if err != nil {
				return err
			}
			info.PostDate = catalog.Products[k.productKey].PostDate
			if info.BuildID == "" {
				return nil
			}
			if _, ok := known[info.BuildID]; ok {
				return nil
			}
			info.InstallType = string(k.InstallType)
			info.ProductKey = k.productKey
			infos = append(infos, *info)
			known[info.BuildID] = struct{}{}
			return nil
		})
	}
	return infos, eg.Wait()
}

type Product struct {
	ExtendedMetaInfo  *ExtendedMetaInfo
	PostDate          time.Time
	Distributions     map[string]string
	ServerMetadataURL string
}

type ExtendedMetaInfo struct {
	ProductType                        string
	ProductVersion                     string
	InstallAssistantPackageIdentifiers *InstallAssistantPackageIdentifiers
}

type InstallAssistantPackageIdentifiers struct {
	InstallInfo string
	OSInstall   string
}

type Catalog struct {
	Products map[string]Product
}

func (c *Catalog) EnglishDistURLForProduct(productKey string) (string, error) {
	product, ok := c.Products[productKey]
	if !ok {
		return "", fmt.Errorf("no such product key in catalog: %q", productKey)
	}
	if distURL, ok := product.Distributions["en"]; ok {
		return distURL, nil
	}
	if distURL, ok := product.Distributions["English"]; ok {
		return distURL, nil
	}
	return "", fmt.Errorf("did not find dist url in product with key %q", productKey)
}

func parseCatalog(r io.Reader) (*Catalog, error) {
	var c Catalog
	err := plist.NewDecoder(r).Decode(&c)
	return &c, errors.Wrap(err, "parsing apple SUS catalog")
}

func findmacOSInstallers(c *Catalog) []pkey {
	var productKeys []pkey
	for key, product := range c.Products {
		if product.ExtendedMetaInfo == nil {
			continue
		}
		if product.ExtendedMetaInfo.ProductType == "macOS" {
			productKeys = append(productKeys, pkey{productKey: key, InstallType: Update})
		}
		if product.ExtendedMetaInfo.InstallAssistantPackageIdentifiers == nil {
			continue
		}
		if product.ExtendedMetaInfo.InstallAssistantPackageIdentifiers.OSInstall == "com.apple.mpkg.OSInstall" {
			productKeys = append(productKeys, pkey{productKey: key, InstallType: OSInstall})
		}
	}
	return productKeys
}

type installType string

const (
	OSInstall installType = "OSInstall"
	Update                = "Update"
)

type pkey struct {
	productKey  string
	InstallType installType
}

func parseDist(r io.Reader) (*InstallerInfo, error) {
	var dinfo distInfo
	if err := xml.NewDecoder(r).Decode(&dinfo); err != nil {
		return nil, errors.Wrap(err, "parsing dist file")
	}
	var info InstallerInfo

	var kv distInfoKV
	if dinfo.AuxInfo.Dict == nil {
		kv = dinfo.AuxInfo.distInfoKV
	} else {
		kv = dinfo.AuxInfo.Dict.distInfoKV
	}
	for i, v := range kv.Keys {
		if v == "VERSION" || v == "macOSProductVersion" {
			info.Version = kv.Values[i]
		}
		if v == "BUILD" || v == "macOSProductBuildVersion" {
			info.BuildID = kv.Values[i]
		}
	}

	for _, c := range dinfo.Choices {
		if strings.Contains(c.SD, "Install macOS") || strings.Contains(c.SD, "Update") || strings.Contains(c.SD, "Developer Beta") {
			info.Title = strings.TrimPrefix(c.SD, "Install ")
		}
	}
	return &info, nil
}

type distInfo struct {
	AuxInfo struct {
		XMLName xml.Name `xml:"auxinfo"`
		Dict    *struct {
			XMLName xml.Name `xml:"dict"`
			distInfoKV
		}
		distInfoKV
	}
	Choices []struct {
		SD string `xml:"suDisabledGroupID,attr"`
	} `xml:"choice"`
}

type distInfoKV struct {
	Keys   []string `xml:"key"`
	Values []string `xml:"string"`
}

type InstallerInfo struct {
	Version     string
	BuildID     string `db:"build_id"`
	Title       string
	InstallType string    `db:"install_type"`
	PostDate    time.Time `db:"post_date"`
	ProductKey  string
}
