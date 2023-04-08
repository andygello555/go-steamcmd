# go-steamcmd

A Go wrapper for the SteamCMD CLI tool. Allows for interaction via SteamCMD's interactive mode as well as command-line arguments.

## Requirements

The `steamcmd` executable must be [installed](https://developer.valvesoftware.com/wiki/SteamCMD#Downloading_SteamCMD), and placed on your PATH as `steamcmd`.

## Status

At the moment the only commands that are supported are:

- `app_info_print`: parses the output data into a `map[string]any` instance.
- `quit`: will wait for the SteamCMD process to terminate.

I only use this module for scraping Steam games, hence the lack of command support for other things. Feel free to make a pull-request with new command implementations!
