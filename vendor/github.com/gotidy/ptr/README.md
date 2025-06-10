# ptr [![GoDoc](https://godoc.org/github.com/gotidy/ptr?status.svg)](https://godoc.org/github.com/gotidy/ptr) [![Go Report Card](https://goreportcard.com/badge/github.com/gotidy/ptr)](https://goreportcard.com/report/github.com/gotidy/ptr) [![Mentioned in Awesome Go](https://awesome.re/mentioned-badge.svg)](https://github.com/avelino/awesome-go)

`ptr` contains functions for simplified creation of pointers from constants of basic types.

Support for generics has been implemented since version 1.4.0. Required 1.18 or later version of Go.

## Installation

`go get github.com/gotidy/ptr`

## Examples

This code:

```go
p := ptr.Of(10)
```

is the equivalent for:

```go
i := int(10)
p := &i  
```

## Documentation

[GoDoc](http://godoc.org/github.com/gotidy/ptr)

## License

[Apache 2.0](https://github.com/gotidy/ptr/blob/master/LICENSE)
