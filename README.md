WIP utility. 
Download all files related to a specific apple product id.


```
Usage of ./upseed install:
  -c string
    	path to catalog plist
  -id string
    	filter by Package ID (default "091-48217")
  -install
    	run the installer command
  -volume string
    	target volume (default "/Volumes/Macintosh HD 1")

Usage of ./upseed check:
  -catalog-path string
    	path to catalog plist
  -catalog-url string
    	catalog URL (default "https://swscan.apple.com/content/catalogs/others/index-10.13customerseed-10.13-10.12-10.11-10.10-10.9-mountainlion-lion-snowleopard-leopard.merged-1.sucatalog")
```

```
sudo /System/Library/PrivateFrameworks/Seeding.framework/Versions/A/Resources/seedutil current |grep CatalogURL

use a mounted (writable) dmg as a target volume
```
