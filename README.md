# goup

A fast, hassle-free Go version updater.

[![Go Version](https://img.shields.io/github/go-mod/go-version/tr1sm0s1n/goup)](./go.mod)
[![Go Reference](https://pkg.go.dev/badge/github.com/tr1sm0s1n/goup.svg)](https://pkg.go.dev/github.com/tr1sm0s1n/goup)
[![Go Report Card](https://goreportcard.com/badge/github.com/tr1sm0s1n/goup)](https://goreportcard.com/report/github.com/tr1sm0s1n/goup)
[![Release](https://img.shields.io/github/v/release/tr1sm0s1n/goup)](https://github.com/tr1sm0s1n/goup/releases)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](./LICENSE.md)

## Installation

Prebuilt binaries:

```sh
curl -sSfL https://raw.githubusercontent.com/tr1sm0s1n/goup/main/install.sh | sh -s -- -b .
```

Using Go:

```sh
go install github.com/tr1sm0s1n/goup@latest
```

## Usage

```sh
# latest version, use '-b' to take backup of the existing version if any
goup

# specific version
goup -i 1.24.7

# goup version
goup -v
```
