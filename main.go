package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/groob/mackit/latest"
	"github.com/groob/plist"
	"github.com/groob/upseed/seed"
)

type Catalog struct {
	Products map[string]Product
}

type Product struct {
	Packages      []Package
	Distributions map[string]string
	PostDate      time.Time
}

type Package struct {
	Digest      string
	URL         string
	MetadataURL string
}

type Language string

func main() {
	if len(os.Args) < 2 {
		os.Exit(1)
	}

	var run func([]string) error
	switch strings.ToLower(os.Args[1]) {
	case "install":
		run = runInstall
	case "check":
		run = runCheck
	default:
		os.Exit(1)
	}

	if err := run(os.Args[2:]); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func runCheck(args []string) error {
	flagset := flag.NewFlagSet("check", flag.ExitOnError)
	var (
		flCatalogURL  = flagset.String("catalog-url", seed.CustomerSeed, "catalog URL")
		flCatalogPath = flagset.String("catalog-path", "", "path to catalog plist")
	)
	if err := flagset.Parse(args); err != nil {
		log.Fatal(err)
	}
	opts := []seed.ClientOption{
		seed.WithCatalogURL(*flCatalogURL),
	}

	if *flCatalogPath != "" {
		opts = append(opts, seed.WithCatalogFromFile(*flCatalogPath))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	client := seed.New(opts...)
	infos, err := client.FindOSInstallers(ctx)
	if err != nil {
		return err
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintf(w, "Version\tBuildID\tTitle\tDate Added\tInstall Type\tProduct Key\n")
	for _, info := range infos {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", info.Version, info.BuildID, info.Title, info.PostDate, info.InstallType, info.ProductKey)
	}
	w.Flush()

	var builds []string
	for _, info := range infos {
		builds = append(builds, info.BuildID)
	}
	fmt.Println("sorted: ", strings.Join(latest.Sorted(builds...), " "))
	return nil
}

func runInstall(args []string) error {
	flagset := flag.NewFlagSet("install", flag.ExitOnError)
	var (
		flCatalog = flagset.String("c", "", "path to catalog plist")
		flPKGID   = flagset.String("id", "091-48217", "filter by Package ID")
		flVolume  = flagset.String("volume", "/Volumes/Macintosh HD 1", "target volume")
		flInstall = flagset.Bool("install", false, "run the installer command")
	)
	if err := flagset.Parse(args); err != nil {
		log.Fatal(err)
	}

	f, err := os.Open(*flCatalog)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	gzr, err := gzip.NewReader(f)
	if err != nil {
		log.Fatal(err)
	}

	var catalog Catalog
	if err := plist.NewDecoder(gzr).Decode(&catalog); err != nil {
		log.Fatal(err)
	}

	product, ok := catalog.Products[*flPKGID]
	if !ok {
		return errors.New("pkgid not found")
	}

	for _, v := range product.Packages {
		dl(v.URL)

		dl(v.MetadataURL)
	}

	for _, dist := range product.Distributions {
		dl(dist)
	}

	rewriteAsTrue(*flPKGID)
	if *flInstall {
		install(fmt.Sprintf("%s.English.dist", *flPKGID), *flVolume)
	}
	return nil

}

func dl(url string) string {
	name := path.Base(url)
	log.Printf("downloading %s\n", name)
	if _, err := os.Stat(name); err == nil {
		log.Printf("using cache for %s\n", name)
		return name
	}
	defer fmt.Println("downloaded ", name)
	client := http.DefaultClient
	resp, err := client.Get(url)
	if err != nil {
		log.Fatalf("downloading %s: %s\n", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Fatalf("downloading %s: got %s\n", url, resp.Status)
	}
	f, err := os.OpenFile(name, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("create file %s: %s\n", name, err)
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		log.Fatal("copy to file ", err)
	}

	return name
}

func install(pkg, volume string) {
	log.Printf("installing %s to volume %s\n", pkg, volume)
	cmd := exec.Command(
		"sudo",
		"/usr/sbin/installer",
		"-pkg", pkg,
		"-target", volume,
	)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	if err := cmd.Run(); err != nil {
		log.Fatalf("installing %s: %s", pkg, err)
	}
}

func rewriteAsTrue(id string) {
	files, err := ioutil.ReadDir("./")
	if err != nil {
		log.Fatal(err)
	}
	for _, info := range files {
		name := filepath.Base(info.Name())
		if name == fmt.Sprintf("%s.English.dist", id) {
			log.Printf("rewriting %s returns to true\n", name)
			f, err := ioutil.ReadFile(info.Name())
			if err != nil {
				log.Fatal(err)
			}

			newFile := bytes.Replace(f, []byte(`return false`), []byte(`return true`), -1)
			if err := ioutil.WriteFile(info.Name(), newFile, info.Mode()); err != nil {
				log.Fatal(err)
			}
		}
	}
}
