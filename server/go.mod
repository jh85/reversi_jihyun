module server

go 1.18

replace game => ../game

require (
	game v0.0.0-00010101000000-000000000000
	go.mongodb.org/mongo-driver v1.9.1
)

require (
	github.com/go-stack/stack v1.8.0 // indirect
	github.com/golang/snappy v0.0.1 // indirect
	github.com/jinzhu/gorm v1.9.16 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/klauspost/compress v1.13.6 // indirect
	github.com/mattn/go-sqlite3 v1.14.12 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/xdg-go/pbkdf2 v1.0.0 // indirect
	github.com/xdg-go/scram v1.0.2 // indirect
	github.com/xdg-go/stringprep v1.0.2 // indirect
	github.com/youmark/pkcs8 v0.0.0-20181117223130-1be2e3e5546d // indirect
	golang.org/x/crypto v0.0.0-20201216223049-8b5274cf687f // indirect
	golang.org/x/exp v0.0.0-20220518171630-0b5c67f07fdf // indirect
	golang.org/x/sync v0.0.0-20190911185100-cd5d95a43a6e // indirect
	golang.org/x/text v0.3.5 // indirect
	gorm.io/driver/sqlite v1.3.2 // indirect
	gorm.io/gorm v1.23.5 // indirect
)
