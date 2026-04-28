.PHONY: build clean

PLUGIN_NAME := claude-cache-control-plugin
OUT_DIR := build
OUT := $(OUT_DIR)/$(PLUGIN_NAME).so

build:
	mkdir -p $(OUT_DIR)
	go build -buildmode=plugin -o $(OUT) main.go

clean:
	rm -rf $(OUT_DIR)
