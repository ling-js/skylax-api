# Skylax-Suite

[![Go Report Card](https://goreportcard.com/badge/github.com/ling-js/skylax-api)](https://goreportcard.com/report/github.com/ling-js/skylax-api)

## Installation

The preferred way for a complete suite installation

nginx proxy:

    1. sudo apt-get install nginx

node:

    1. https://nodejs.org/en/download/package-manager/
    2. sudo apt-get install npm
    3. npm install pm2 -g // cluster management

### Starting
    1. pm2 start app.js -i 4

## skylax-client
### Installation

    1. git clone https://github.com/ling-js/skylax.git
    2. cd skylax
    3. npm install
    4. npm start

###


## skylax-api
### Requirements
The skylax-api depends on the following libraries to be installed in the host operating system. Please refer to the their Websites for installation instructions:
 * [GEOS](http://trac.osgeo.org/geos/) released under [LGPL](https://git.osgeo.org/gitea/geos/geos/src/branch/master/COPYING)
 * [proj.4](https://github.com/OSGeo/proj.4) released under [MIT License](http://proj4.org/license.html)

### Installation
Run following commands in the desired install directory

    TODO(specki): gorelease
    1. go get github.com/ling-js/skylax-api
    2. go build

### Running
Run following commands in the install directory

`./skylax-api`

## License
see `LICENSE`
## Dependencies
The skylax-api has the following external dependencies:

 * [go-gdal](github.com/ling-js/go-gdal) released under [License](https://github.com/ling-js/go-gdal/blob/master/LICENSE)
 * [geos](github.com/paulsmith/gogeos/geos) released under [MIT License](https://github.com/paulsmith/gogeos/blob/master/COPYING)
 * [gorilla](github.com/gorilla/schema) released under [BSD-3-Clause](https://github.com/gorilla/schema/blob/master/LICENSE)
 * [ksuid](github.com/segmentio/ksuid) released under [MIT License](https://github.com/segmentio/ksuid/blob/master/LICENSE.md)
 * gdal2tiles.py released under (see file)


