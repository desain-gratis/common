module github.com/desain-gratis/common/example/user-profile

go 1.23.4

replace github.com/desain-gratis/common => ../..

require (
	github.com/jmoiron/sqlx v1.4.0
	github.com/julienschmidt/httprouter v1.3.0
	github.com/rs/zerolog v1.33.0
)

require (
	github.com/desain-gratis/common v0.0.0-20250714193823-0653a050800a // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.19 // indirect
	golang.org/x/sys v0.33.0 // indirect
)
