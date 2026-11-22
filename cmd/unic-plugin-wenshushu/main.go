package main

import (
	share "github.com/UniClipboard/unic-plugin-wenshushu"
	"github.com/uniclipboard/unic/plugin"
	"os"
)

func main() {
	if plugin.Serve(share.New(nil)) != nil {
		os.Exit(1)
	}
}
