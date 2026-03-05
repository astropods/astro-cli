package main

import "github.com/postman/astro/apps/astro-queen/cmd"

func main() {
	cmd.WebFS = webFS
	cmd.OpenAPIJSON = openapiJSON
	cmd.Execute()
}
