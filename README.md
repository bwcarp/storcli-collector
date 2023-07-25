# node-exporter-storcli.go
A port of node-exporter-textfile-collector-scripts/storcli.py to Go

This is a drop-in replacement for the storcli.py collector. 

**NOTE:** The `mpt3sas` driver is _NOT_ yet supported. You can help! If you have a RAID host using this driver, send me your json output and I'll use it to test. 
```
storcli /cALL show all J
```

An additional option `--outfile` is available in this version. This will write to a text file instead of standard out in the event you are using this as a cron.

No Makefile is provided, just use go build.
```
go build storcli-collector.go
```

**This is a work in progress.** If you receive errors or things are not parsing correctly, please provide the json output in your issue so that it can be used for local testing. You may also use the email link on my profile.