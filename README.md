# dbin: The easy to use, easy to get, suckless software distribution system
[![Go Report Card](https://goreportcard.com/badge/github.com/xplshn/dbin)](https://goreportcard.com/report/github.com/xplshn/dbin)
[![License](https://img.shields.io/badge/license-%20RABRMS-bright_green)](https://github.com/xplshn/dbin/blob/master/LICENSE)
[![GitHub release (latest by date including pre-releases)](https://img.shields.io/github/v/release/xplshn/dbin?include_prereleases)](https://github.com/xplshn/dbin/releases/latest)
![GitHub code size in bytes](https://img.shields.io/github/languages/code-size/xplshn/dbin)

<p align="center"><img src="https://github.com/user-attachments/assets/3c2dd460-6590-4e69-9c08-69bcccf77d9d" alt="dbin logo, made by a proffesional (my brother)" width="150" /></p>

<!--[Makes my repo look bad because these usually show "Failling"]-------------------------------------------------------------------------------------------
[![AMD64 repo status](https://github.com/Azathothas/Toolpacks/actions/workflows/build_x86_64_Linux.yaml/badge.svg)](https://github.com/Azathothas/Toolpacks)
[![ARM64 repo status](https://github.com/Azathothas/Toolpacks/actions/workflows/build_aarch64_Linux.yaml/badge.svg)](https://github.com/Azathothas/Toolpacks)
-->

dbin is a simple, Golang-based rewrite of the original [BDL](https://github.com/xplshn/Handyscripts/blob/master/bdl), it is like a package manager, but without the hassle of dependencies nor the bloat, every binary provided is statically linked. This tool is made to operate on Linux systems, with plans to expand to other platforms soon, dbin is particularly well-suited for embedded systems, we support both amd64 & aarch64. (freeBSD + linuxlator is supported and works quite wonderfully, specially if you want an embedded-ready freeBSD install, you can pair it with `dbin` instead of `pkg`)

> Why?

 “I tend to think the drawbacks of dynamic linking outweigh the advantages for many (most?) applications.” – John Carmack. If you are looking for more in-depth arguments, see: [cat-v.ORG - Dynamic Linking](https://harmful.cat-v.org/software/dynamic-linking)

> I've seen lots of package manager projects without "packages". What is different about this one?

 There are currently ![Current amount of binaries in the repos! x86_64](https://raw.githubusercontent.com/xplshn/dbin/master/misc/assets/counter.svg) binaries in our repos. They are all statically linked.

### Features ![pin](https://raw.githubusercontent.com/xplshn/dbin/master/misc/assets/pin.svg)

```
$ dbin --help

 Copyright (c) 2025: xplshn and contributors
 For more details refer to https://github.com/xplshn/dbin

  Synopsis
    dbin [-v|-h] [list|install|remove|update|run|info|search|tldr|eget2] <-args->
  Description:
    The easy to use, easy to get, software distribution system
  Options:
    -h, --help        Show this help message
    -v, --version     Show the version number
  Commands:
    list              List all available binaries
    install, add      Install a binary
    remove, del       Remove a binary
    update            Update binaries, by checking their SHA against the repo's SHA
    run               Run a specified binary from cache
    info              Show information about a specific binary OR display installed binaries
    search            Search for a binary - (not all binaries have metadata. Use list to see all binaries)
    tldr              Equivalent to "--silent run --transparent tlrc"
    eget2             Equivalent to "--silent run --transparent eget2"
  Variables:
    DBIN_CACHEDIR      If present, it must contain a valid directory path
    DBIN_INSTALL_DIR   If present, it must contain a valid directory path
    DBIN_NOTRUNCATION  If present, and set to ONE (1), string truncation will be disabled
    DBIN_REOWN         If present, and set to ONE (1), it makes dbin update programs that may not have been installed by dbin
    DBIN_REPO_URLS     If present, it must contain one or more repository URLS ended in / separated by ;
    DBIN_METADATA_URLS If present, it must contain one or more repository's metadata url separated by ;

```

### Examples
```
    dbin search editor
    dbin install micro.upx
    dbin install lux kakoune aretext shfmt
    dbin --silent install bed && echo "[bed] was installed to $INSTALL_DIR/bed"
    dbin del bed
    dbin del orbiton tgpt lux
    dbin info
    dbin info | grep a-utils | xargs dbin add # install the entire a-utils suite
    dbin info jq
    dbin list --described
    dbin tldr gum
    dbin --verbose run curl -qsfSL "https://raw.githubusercontent.com/xplshn/dbin/master/stubdl" | sh -
    dbin --silent run elinks -no-home "https://fatbuffalo.neocities.org/def"
    dbin --silent run --transparent micro ~/.profile
    dbin run firefox "https://www.paypal.com/donate/?hosted_button_id=77G7ZFXVZ44EE" # Donate?
```

#### What are these optional flags? ![pin](https://raw.githubusercontent.com/xplshn/dbin/master/misc/assets/pin.svg)
##### Flags that correspond to the `run` functionality
In the case of `--transparent`, it runs the program from $PATH and if it isn't available in the user's $PATH it will pull the binary from `dbin`'s repos and run it from cache.
In the case of `--silent`, it simply hides the progressbar and all optional messages (warnings) that `dbin` can show, which would always report if the binary is found on cache + the return code of the binary to be run if it differs from 0 otherwise.
##### Flags that correspond to the `install` functionality
`--silent`, it hides the progressbar and doesn't print the installation message
##### `Update` arguments:
Update can receive an optional list of specific binaries to update OR no arguments at all. When `update` receives no arguments it updates everything that is both found in the repos and in your `$DBIN_INSTALL_DIR`.
##### Arguments of `info`
When `info` is called with no arguments, it displays binaries which are part of the `list` and are also found on your `$DBIN_INSTALL_DIR`. If `info` is called with a binary's name as argument, `info` will display as much information of it as is available. The "Size", "SHA256", "Version" fields may not match your local installation if the binary wasn't provided by `dbin` or if it isn't up-to-date.
###### Example:
```
$ dbin info micro
Name: micro
Description: A modern and intuitive terminal-based text editor
Version: v2.0.14
Download URL: https://bin.pkgforge.dev/x86_64/micro
Size: 11.67 MB
B3SUM: 2455db4db6e117717b33f6fb4a85d6630268442b111e1012e790feae6255484a
SHA256: 6be82c65571f6aac935e7ef723932322ed5d665028a2179d66211b5629d4b665
Build Date: 2024-08-31T01:08:46
Source URL: https://github.com/zyedidia/micro
Web URL: https://github.com/zyedidia/micro
Build Script: https://github.com/Azathothas/Toolpacks/tree/main/.github/scripts/x86_64_Linux/bins/micro.sh
Build Log: https://bin.pkgforge.dev/x86_64/micro.log.txt
Category: command-line, cross-platform, editor, go, golang, micro, terminal, text-editor
```
##### Arguments of `list`
`list` can receive the optional argument `--described`/`-d`. It will display all binaries that have a description in their metadata.
##### Arguments of `search`
`search` can only receive ONE search term, if the name of a binary or a description of a binary contains the term, it is shown as a search result.
`search` can optionally receive a `--limit` argument, which changes the limit on how many search results can be displayed (default is 90).

## Getting Started ![pin](https://raw.githubusercontent.com/xplshn/dbin/master/misc/assets/pin.svg)

To begin using dbin, simply run one of these commands on your Linux system. No additional setup is required. You may also build the project using `go build or go install`
#### Use without installing
```
wget -qO- "https://raw.githubusercontent.com/xplshn/dbin/master/stubdl" | sh -s -- --help
```
##### Install to `~/.local/bin`
```
wget -qO- "https://raw.githubusercontent.com/xplshn/dbin/master/stubdl" | sh -s -- --install "$HOME/.local/bin/dbin"
```

### Examples of usage cases of `dbin`
#### Inside of a SH script
Whenever you want to pull a specific GNU coreutil, busybox, toybox, etc, insert a bash snippet, use a *fetch tool, etc, you can use dbin for the job! There's also a `--transparent` flag for `run`, which will use the users' installed version of the program you want to run, and if it is not found in the `$PATH` dbin will fetch it and run it from `$DBIN_CACHEDIR`.
```sh
system_info=$(wget -qO- "https://raw.githubusercontent.com/xplshn/dbin/master/stubdl" | sh -s -- run --silent albafetch --no-logo - || curl -qsfSL "https://raw.githubusercontent.com/xplshn/dbin/master/stubdl" | sh -s -- run --silent albafetch --no-logo -)
```
#### For creating a statically-linked & bootable rootfs using `toybox`'s `mkroot`
![image](https://github.com/user-attachments/assets/949465ab-9572-404f-b02d-319eb3bc2fe0)
![image](https://github.com/user-attachments/assets/64700072-a087-4206-8b52-212ecfea668d)

### Where do these binaries come from? ![pin](https://raw.githubusercontent.com/xplshn/dbin/master/misc/assets/pin.svg)
- [AppBundleHub](https://github.com/xplshn/AppBundleHUB)
- [PkgForge's repos](https://docs.pkgforge.dev/repositories)

> Hmm, can I add my own repos?

Yes! Absolutely. The repo's URL's are declared in main.go. Its simply a matter of providing a repo URL in the same format that the [PkgForge Project uses](https://docs.pkgforge.dev/repositories/pkgforge-edge/metadata). You may skip the metadata part if you're only interested in the `install/add` functionality.

>Good to hear, now... What about the so-called MetadataURLs?

MetadataURLs provide info about the binaries, which is used to `search` and `update` binaries, also for the functionality of `info` in both of its use-cases (showing the binaries which were installed to $DBIN_INSTALL_DIR from the [PkgForge](https://github.com/pkgforge) [repositories](https://docs.pkgforge.dev/repositories)) and showing a binary's description, size, etc. You can take a look at [`modMetadata`'s](misc/cmd/modMetadata/main.go) `Item` struct if you want to make a custom repo which's binaries appear in `search`, are compatible with the `update` functionality and also work with `info`.

### Libraries
I am using these two libraries for `dbin`:
1. https://github.com/schollz/progressbar
2. https://github.com/goccy/go-json

## Contributing
Contributions are welcome! Whether you've found a bug, have a feature request, or wish to improve the documentation, your input is valuable. Fork the repository, make your changes, and submit a pull request. Together, we can make dbin even more powerful and simpler. If you can provide repos that meet the requirements to add them to `dbin`, I'd be grateful.
Also, I need help optimizing the cyclomatic complexity of `dbin`.

## License
dbin is licensed under the ISC or RABRMS Licenses, choose whichever fits your needs best.

## Its pretty safe to state that we are ![cooltext466498248029130](https://github.com/user-attachments/assets/4397b1d3-44f2-4ae9-99c6-7379860bfa73)
