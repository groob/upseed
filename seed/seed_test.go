package seed

import (
	"os"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_parseCatalog(t *testing.T) {
	getCatalog(t)
}

func Test_findmacOSInstaller(t *testing.T) {
	catalog := getCatalog(t)

	validKeys := []string{"091-31306", "091-52054", "091-52056", "091-62779", "091-62780", "091-70035", "091-70036", "091-71284", "091-71617"}
	products := findmacOSInstallers(catalog)
	var productKeys []string
	for _, p := range products {
		productKeys = append(productKeys, p.productKey)
	}
	sort.Strings(productKeys)
	assert.Equal(t, validKeys, productKeys)
}

func Test_EnglishDistURLForProductKey(t *testing.T) {
	catalog := getCatalog(t)

	tests := map[string]string{
		"091-62780": "https://swdist.apple.com/content/downloads/10/36/091-62780/objx55fn5lwnefnetcap2i0c7xg3avoor9/091-62780.English.dist",
		"091-62779": "https://swdist.apple.com/content/downloads/16/14/091-62779/frfttxz116hdm02ajg89z3cubtiv64r39s/091-62779.English.dist",
		"091-71284": "https://swdist.apple.com/content/downloads/45/61/091-71284/77pnhgsj5oza9h28y7vjjtby8s1binimnj/091-71284.English.dist",
	}

	for productKey, distURL := range tests {
		t.Run(productKey, func(t *testing.T) {
			foundDistURL, err := catalog.EnglishDistURLForProduct(productKey)
			require.NoError(t, err)
			assert.Equal(t, distURL, foundDistURL)
		})
	}
}

func getCatalog(t *testing.T) *Catalog {
	t.Helper()
	f, err := os.Open("testdata/sucatalog_01C7PBW3JYWAY8AA0H6R1BT3XE.plist")
	require.NoError(t, err)
	defer f.Close()

	catalog, err := parseCatalog(f)
	require.NoError(t, err)

	return catalog
}

func Test_parseDist(t *testing.T) {
	var tests = []struct {
		distPath string
		Title    string
		Version  string
		BuildID  string
	}{
		{
			distPath: "testdata/091-71617.English.dist",
			Version:  "10.13.3",
			BuildID:  "17D2104",
			Title:    "macOS High Sierra 10.13.3 Supplemental Update",
		},
		{
			distPath: "testdata/091-62780.English.dist",
			Version:  "10.13.3",
			BuildID:  "17D47",
			Title:    "macOS High Sierra",
		},
		{
			distPath: "testdata/091-62779.English.dist",
			Version:  "10.13.3",
			BuildID:  "17D2047",
			Title:    "macOS High Sierra",
		},
		{
			distPath: "testdata/091-71284.English.dist",
			Version:  "10.13.4",
			BuildID:  "17E160g",
			Title:    "macOS High Sierra Beta",
		},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			f, err := os.Open(tt.distPath)
			require.NoError(t, err)
			defer f.Close()

			info, err := parseDist(f)
			require.NoError(t, err)

			assert.Equal(t, tt.Title, info.Title)
			assert.Equal(t, tt.Version, info.Version)
			assert.Equal(t, tt.BuildID, info.BuildID)
		})
	}
}
