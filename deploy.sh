#!/bin/sh

go build server.go searchHandler.go generateHandler.go && scp -P 14011 server j_buss16@gis-bigdata.uni-muenster.de:/home/j_buss16/skylax-api
