# BigDL: Advanced Binary Management Tool
[![Go Report Card](https://goreportcard.com/badge/github.com/xplshn/bigdl)](https://goreportcard.com/report/github.com/xplshn/bigdl)
[![License](https://img.shields.io/badge/license-%20RABRMS-bright_green)](https://github.com/xplshn/bigdl/blob/master/LICENSE)
[![GitHub release (latest by date including pre-releases)](https://img.shields.io/github/v/release/xplshn/bigdl?include_prereleases)](https://github.com/xplshn/bigdl/releases/latest)
![GitHub code size in bytes](https://img.shields.io/github/languages/code-size/xplshn/bigdl)

<!--[Makes my repo look bad because these usually show "Failling"]-------------------------------------------------------------------------------------------
[![AMD64 repo status](https://github.com/Azathothas/Toolpacks/actions/workflows/build_x86_64_Linux.yaml/badge.svg)](https://github.com/Azathothas/Toolpacks)
[![ARM64 repo status](https://github.com/Azathothas/Toolpacks/actions/workflows/build_aarch64_Linux.yaml/badge.svg)](https://github.com/Azathothas/Toolpacks)
-->

BigDL is a sophisticated, Golang-based rewrite of the original [BDL](https://github.com/xplshn/Handyscripts/blob/master/bdl), it is like a package manager, but without the hassle of dependencies nor the bloat, every binary provided is statically linked. This tool is made to operate on Linux systems, BigDL is particularly well-suited for embedded systems, with support for both Amd64 AND Aarch64. Optionally, it works under Android too, but you'll have to set $INSTALL_DIR and $BIGDL_CACHE if you aren't running it under Termux, since depending the Android version and the ROM used, directories vary and the user's permission to modify them too.

> Why?

 “I tend to think the drawbacks of dynamic linking outweigh the advantages for many (most?) applications.” – John Carmack

> I've seen lots of package manager projects without "packages". What is different about this one?

 There are currently ![Current amount of binaries in the repos! x86_64](https://raw.githubusercontent.com/xplshn/bigdl/master/misc/assets/counter.svg) binaries in our repos. They are all statically linked.

### Features ![pin](https://raw.githubusercontent.com/xplshn/bigdl/master/misc/assets/pin.svg)

```
$ bigdl --help
Usage:
 [-v|-h] [list|install|remove|update|run|info|search|tldr] <{args}>

Options:
 -h, --help       Show this help message
 -v, --version    Show the version number

Commands:
 list             List all available binaries
 install, add     Install a binary to $INSTALL_DIR
 remove, del      Remove a binary from the $INSTALL_DIR
 update           Update binaries, by checking their SHA against the repo's SHA
 run              Run a binary from cache
 info             Show information about a specific binary OR display installed binaries
 search           Search for a binary - (not all binaries have metadata. Use list to see all binaries)
 tldr             Show a brief description & usage examples for a given program/command. This is an alias equivalent to using "run" with "tlrc" as argument.
```

### Examples
```
 bigdl search editor
 bigdl install micro
 bigdl install lux kakoune aretext shfmt
 bigdl install --silent bed && echo "[bed] was installed to $INSTALL_DIR/bed"
 bigdl del bed
 bigdl del orbiton tgpt lux
 bigdl info
 bigdl info jq
 bigdl list --described
 bigdl tldr gum
 bigdl run --verbose curl -qsfSL "https://raw.githubusercontent.com/xplshn/bigdl/master/stubdl" | sh -
 bigdl run --silent elinks -no-home "https://fatbuffalo.neocities.org/def"
 bigdl run --transparent --silent micro ~/.profile
 bigdl run btop
```

#### What are these optional flags? ![pin](https://raw.githubusercontent.com/xplshn/bigdl/master/misc/assets/pin.svg)
##### Flags that correspond to the `run` functionality
In the case of `--transparent`, it runs the program from $PATH and if it isn't available in the user's $PATH it will pull the binary from `bigdl`'s repos and run it from cache.
In the case of `--silent`, it simply hides the progressbar and all optional messages (warnings) that `bigdl` can show, as oppossed to `--verbose`, which will always report if the binary is found on cache + the return code of the binary to be ran if it differs from 0.
##### Flags that correspond to the `install` functionality
`--silent`, it hides the progressbar and doesn't print the installation message
##### `Update` arguments:
Update can receive an optional list of specific binaries to update OR no arguments at all. When `update` receives no arguments it updates everything that is both found in the repos and in your `$INSTALL_DIR`.
##### Arguments of `info`
When `info` is called with no arguments, it displays binaries which are part of the `list` and are also found on your `$INSTALL_DIR`. If `info` is called with a binary's name as argument, `info` will display as much information of it as is available. The "Size", "SHA256", "Version" fields may not match your local installation if the binary wasn't provided by `bigdl` or if it isn't up-to-date.
###### Example:
```
$ bigdl info micro
Name: micro
Description: A modern and intuitive terminal-based text editor
Repo: https://github.com/zyedidia/micro
Updated: 2024-05-22T20:21:10Z
Version: v2.0.13
Size: 11.81 MB
Source: https://bin.ajam.dev/x86_64_Linux/micro
SHA256: 697fb918c800071c4d1a853d515331a9a3f245bb8a7da1c6d3653737d17ce3c4
```
##### Arguments of `list`
`list` can receive the optional argument `--described`/`-d`. It will display all binaries that have a description in their metadata.
##### Arguments of `search`
`search` can only receive ONE search term, if the name of a binary or a description of a binary contains the term, it is shown as a search result.
`search` can optionally receive a `--limit` argument, which changes the limit on how many search results can be displayed (default is 90).

## Getting Started ![pin](https://raw.githubusercontent.com/xplshn/bigdl/master/misc/assets/pin.svg)

To begin using BigDL, simply run one of these commands on your Linux system. No additional setup is required. You may also build the project using `go build or go install`
#### Use without installing
```
wget -qO- "https://raw.githubusercontent.com/xplshn/bigdl/master/stubdl" | sh -s -- --help
```
##### Install to `~/.local/bin`
```
wget -qO- "https://raw.githubusercontent.com/xplshn/bigdl/master/stubdl" | sh -s -- --install "$HOME/.local/bin/bigdl"
```

#### Example of one use case of bigdl | Inside of a SH script
Whenever you want to pull a specific GNU coreutil, busybox, toybox, etc, insert a bash snippet, use a *fetch tool, etc, you can use bigdl for the job! There's also a `--transparent` flag for `run`, which will use the users' installed version of the program you want to run, and if it is not found in the `$PATH` bigdl will fetch it and run it from `/tmp/bigdl_cached`.
```sh
system_info=$(wget -qO- "https://raw.githubusercontent.com/xplshn/bigdl/master/stubdl" | sh -s -- run --silent albafetch --no-logo - || curl -qsfSL "https://raw.githubusercontent.com/xplshn/bigdl/master/stubdl" | sh -s -- run --silent albafetch --no-logo -)
```

### Where do these binaries come from? ![pin](https://raw.githubusercontent.com/xplshn/bigdl/master/misc/assets/pin.svg)
- https://github.com/Azathothas/Toolpacks [https://bin.ajam.dev] [https://bin.ajam.dev/*/Baseutils/]
>Hmm, can I add my own repos?

Yes! Absolutely. The repo's URL's are declared in main.go, simply add another one if your repo is hosted at Github or your endpoint follows the same JSON format that Github's endpoint provides. You can also provide a repo URL in the same format that the [Toolpacks](https://github.com/Azathothas/Toolpacks) repo uses.

>Good to hear, now... What about the so-called MetadataURLs?

MetadataURLs provide info about the binaries, which is used to `search` and update `binaries`, also for the functionality of `info` in both of its use-cases (showing the binaries which were installed to $INSTALL_DIR from the [Toolpacks](https://github.com/Azathothas/Toolpacks) repo) and showing a binary's description, size, etc.

## NOTE
A rewrite of `bigdl` from start to finish is underway. Applying the Data-Oriented paradigm, in a procedural/functional way, avoiding global variables and race conditions. (0.1/1)
It will be release [One of These Days](https://music.youtube.com/watch?v=48PJGVf4xqk)...

### Libraries
I am using these two libraries for `bigdl`:
1. https://github.com/schollz/progressbar
2. https://github.com/goccy/go-json

## Contributing
Contributions are welcome! Whether you've found a bug, have a feature request, or wish to improve the documentation, your input is valuable. Fork the repository, make your changes, and submit a pull request. Together, we can make BigDL even more powerful and simpler. If you can provide repos that meet the requirements to add them to `bigdl`, I'd be grateful.
Also, I need help optimizing the cyclomatic complexity of `bigdl`.

## License
BigDL is licensed under the RABRMS License. This allows for the use, modification, and distribution of the software under certain conditions. For more details, please refer to the [LICENSE](LICENSE) file. This license is equivalent to the New or Revised BSD License.
