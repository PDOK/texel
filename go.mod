module github.com/woutervisscher/sieve

go 1.17

require (
	github.com/go-spatial/geom v0.0.0-20210110002716-a43924ed9afb
	github.com/mattn/go-sqlite3 v1.14.6 // indirect
)

require github.com/gdey/errors v0.0.0-20190426172550-8ebd5bc891fb // indirect

replace github.com/go-spatial/geom => github.com/pdok/geom v0.0.0-20220126094349-4bbd2041904e
