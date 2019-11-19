SET APP_DIR=build
SET GOARCH=386
SET PACKAGE_TO_BUILD=github.com/fpawel/seh3dak6015/cmd/6015
go build -o %APP_DIR%\6015.exe %PACKAGE_TO_BUILD%