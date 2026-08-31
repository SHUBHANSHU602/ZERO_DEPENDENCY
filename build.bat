@echo off
setlocal
set "OUTPUT=%~1"
if "%OUTPUT%"=="" set "OUTPUT=pulselog.exe"

go version > "%OUTPUT%.go-version.txt"
if errorlevel 1 exit /b %errorlevel%
go version
go build -trimpath -buildvcs=false -o "%OUTPUT%" ./cmd/pulselog
exit /b %errorlevel%
