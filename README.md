# dbin: The easy to use, easy to get, suckless software distribution system
[![Go Report Card](https://goreportcard.com/badge/github.com/xplshn/dbin)](https://goreportcard.com/report/github.com/xplshn/dbin)
[![License](https://img.shields.io/badge/license-%20RABRMS-bright_green)](https://github.com/xplshn/dbin/blob/master/LICENSE)
[![GitHub release (latest by date including pre-releases)](https://img.shields.io/github/v/release/xplshn/dbin?include_prereleases)](https://github.com/xplshn/dbin/releases/latest)
![GitHub code size in bytes](https://img.shields.io/github/languages/code-size/xplshn/dbin)

<p align="center"><img src="https://github.com/user-attachments/assets/3c2dd460-6590-4e69-9c08-69bcccf77d9d" alt="dbin logo, made by a professional (my brother)" width="150" /></p>

<!--[Makes my repo look bad because these usually show "Failing"]-------------------------------------------------------------------------------------------
[![AMD64 repo status](https://github.com/Azathothas/Toolpacks/actions/workflows/build_x86_64_Linux.yaml/badge.svg)](https://github.com/Azathothas/Toolpacks)
[![ARM64 repo status](https://github.com/Azathothas/Toolpacks/actions/workflows/build_aarch64_Linux.yaml/badge.svg)](https://github.com/Azathothas/Toolpacks)
-->

dbin is a simple, Golang-based rewrite of the original [BDL](https://github.com/xplshn/Handyscripts/blob/master/bdl), it is like a package manager, but without the hassle of dependencies nor the bloat, every binary provided is statically linked. This tool is made to operate on Linux systems, with plans to expand to other platforms soon, dbin is particularly well-suited for embedded systems, we support both amd64 & aarch64. (freeBSD + linuxlator is supported and works quite wonderfully, especially if you want an embedded-ready freeBSD install, you can pair it with `dbin` instead of `pkg`)

> Why?

 "I tend to think the drawbacks of dynamic linking outweigh the advantages for many (most?) applications." – John Carmack. If you are looking for more in-depth arguments, see: [cat-v.ORG - Dynamic Linking](https://harmful.cat-v.org/software/dynamic-linking)

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
    search            Search for a binary by supplying one or more search terms
  Variables:
    DBIN_CACHEDIR      If present, it must contain a valid directory path
    DBIN_INSTALL_DIR   If present, it must contain a valid directory path
    DBIN_NOTRUNCATION  If present, and set to ONE (1), string truncation will be disabled
    DBIN_REOWN         If present, and set to ONE (1), it makes dbin update programs that may not have been installed by dbin
    DBIN_NOCONFIG      If present, and set to ONE (1), it makes dbin use its builtin config, it won't create or read an existing one
    DBIN_REPO_URLS     If present, it must contain one or more repository's index file urls separated by ;

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

#### Features ![pin](https://raw.githubusercontent.com/xplshn/dbin/master/misc/assets/pin.svg)
- Ability to install specific versions of binaries (by specifying the Snapshot you want to install of it)
- Ability to use user-curated repositories, as the format for the repository indexes is quite simple
  Note that a repo index for `dbin` can be in either of these formats:
  - JSON: For ease of use
  - CBOR: To optimize the time it takes to load the repo
  - YAML: Because this library is already used for the config, so, why not?
   The repo indexes can be compressed as .gz or .zst, this is specially useful for large catalogs of programs
- Hooks. `dbin` can run a set of commands or a script, whenever a binary of file with a certain extension is being installed. The user can also specify uninstall hooks
- `dbin` can work without a config, and it can also run from RAM
- No breaking changes. Old releases of `dbin` do not break, until after at least 3 releases of newer versions.
- `dbin` will survive even if the upstream repo disappears. `dbin` has a repository index that is held in its own repo. And given that the upstream binaries are held & built in GHCR, all build logs and binaries will continue to be usable/downloadable even if `pkgforge` disappears or experiences downtime
- The binaries in `dbin` are specifically filtered for `portability` (meaning, they work on older kernel versions, they work on `Musl` and `Glibc` and even weird yet wonderful systems like [AliceLinux](https://github.com/xplshn/alicelinux), [OasisLinux](https://github.com/oasislinux/oasis) or FreeBSD (with linuxlator, so that you don't have to set up an Ubuntu chroot to install a Linux web browser and be able to watch DRM content, etc)

#### What are these optional flags? ![pin](https://raw.githubusercontent.com/xplshn/dbin/master/misc/assets/pin.svg)
##### Flags that correspond to the `run` functionality
In the case of `--transparent`, it runs the program from $PATH and if it isn't available in the user's $PATH it will pull the binary from `dbin`'s repos and run it from cache.
##### `Update` arguments:
Update can receive an optional list of specific binaries to update OR no arguments at all. When `update` receives no arguments it updates everything that is both found in the repos and in your `$DBIN_INSTALL_DIR` (unless `$DBIN_REOWN=1`, dbin will not update binaries it didn't install. It won't modify symlinks either).
##### Arguments of `info`
When `info` is called with no arguments, it displays all binaries which were installed by `dbin`. When it is called with a binary's name as argument, `info` will display as much information of it as is available.
###### Example:
```
$ dbin info micro
Name: micro#github.com.zyedidia.micro
Pkg ID: github.com.zyedidia.micro
Pretty Name: micro
Description: Modern and intuitive terminal-based text editor
Version: HEAD-7d16dcd-250311T073524
Download URL: oci://ghcr.io/pkgforge/bincache/micro/official/micro:HEAD-7d16dcd-250311T073524-x86_64-linux
Size: 14 MB
B3SUM: b6f32b2b8ff4b5abad126c4d0d466549f91c12eedafbc9826d0673b91d10eea9
SHA256: 3a9278b2be59a8c1fd87647f65d0cfcb9c707e823713045e93af183610ef8102
Build Date: 2025-03-12T03:02:06Z
Build Script: https://github.com/pkgforge/soarpkgs/blob/main/binaries/micro/static.official.source.yaml
Build Log: https://api.ghcr.pkgforge.dev/pkgforge/bincache/micro/official/micro?tag=HEAD-7d16dcd-250311T073524-x86_64-linux&download=micro.log
Rank: 1102
Snapshots: HEAD-9b3f7ff-250119T130748-x86_64-linux [2.0.14]
```
##### Arguments of `list`
`list` can receive the optional argument `--described`/`-d`. It will display all binaries + their description
##### Arguments of `search`
`search` can only receive various search terms, if the name of a binary or a description of a binary contains the term, it is shown as a search result.
`search` can optionally receive a `--limit` argument, which changes the limit on how many search results can be displayed (default is 90) (you can also put this in your config)

## Getting Started ![pin](https://raw.githubusercontent.com/xplshn/dbin/master/misc/assets/pin.svg)

To begin using dbin, simply run one of these commands on your Linux/FreeBSD+Linuxlator system. No additional setup is required. You may also build the project using `go build` or `go install`
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
   - Note however that pkgforge also has dynamic (unportable) programs/packages (that only run on Debian & derivatives), `dbin` filters these out, leaving behind only the static/portable programs.

> Hmm, can I add my own repos?

Yup, you'll need to provide a repository index, a JSON (or CBOR/YAML) (it can optionally be compressed | as .gz or .zst) file like this:
```json
{
 "SillyRepo": [
  {
   "pkg": "a-utils/ed",
   "pkg_name": "ed",
   "pkg_id": "github.com.xplshn.a-utils",
   "icon": "https://api.ghcr.pkgforge.dev/pkgforge/bincache/a-utils/official/ed?tag=0.0.5-x86_64-linux\u0026download=ed.png",
   "description": "ed from u-root but with syntax highlighting",
   "version": "0.0.5",
   "download_url": "oci://ghcr.io/pkgforge/bincache/a-utils/official/ed:0.0.5-x86_64-linux",
   "size": "5.4 MB",
   "bsum": "1f0213b604bf964dbd2ad0dbaddab1c6a64e5da7ea89febcbdb9de337a081d19",
   "shasum": "e2deb5cb912858603900a890630876e0830a9a912d2a71c05742b697c98bc768",
   "build_date": "2025-01-21T02:21:53Z",
   "src_urls": [
    "https://github.com/xplshn/a-utils"
   ],
   "web_urls": [
    "https://github.com/xplshn/a-utils",
    "https://github.com/xplshn/Andes"
   ],
   "build_script": "https://github.com/pkgforge/soarpkgs/blob/main/binaries/a-utils/static.official.stable.yaml",
   "build_log": "https://api.ghcr.pkgforge.dev/pkgforge/bincache/a-utils/official/ed?tag=0.0.5-x86_64-linux\u0026download=ed.log",
   "snapshots": [
    {
     "commit": "0.0.5-x86_64-linux",
     "version": "0.0.5"
    }
   ],
   "notes": [
    "Pre Built Binary Fetched from Upstream. Check/Report @ https://github.com/xplshn/a-utils"
   ],
   "rank": 1012
  },
... More Entries here ...
}
```

NOTE: Not all fields are essential :)

A schema of the metadata format can be found here [/misc/cmd/dbinRepoIndexGenerators/*/generator.go](https://github.com/xplshn/dbin/tree/master/misc/cmd/dbinRepoIndexGenerators)

### Libraries
I am using these libraries for `dbin`:
1. https://github.com/urfave/cli (v3)
2. https://github.com/hedzr/progressbar (an attractive-looking progressbar, but with a messy API and codebase. Currently the only good alternative to mpb/v8+)
3. https://github.com/goccy/go-json (index files can be .json)
4. https://github.com/goccy/go-yaml (config && index files can be .yaml)
5. https://github.com/klauspost/compress (index files can be .zst && .gz)
6. https://github.com/fxamacker/cbor (index files can be .cbor)
7. https://github.com/zeebo/blake3 (checksums)
8. https://golang.org/x/term

## Contributing
Contributions are welcome! Whether you've found a bug, have a feature request, or wish to improve the documentation, your input is valuable. Fork the repository, make your changes, and submit a pull request. Together, we can make dbin even more powerful and simpler. If you can provide repos that meet the requirements to add them to `dbin`, I'd be grateful.

## License
dbin is licensed under the ISC (equivalent to MIT but without unnecessary legal wording) or RABRMS Licenses, choose whichever fits your needs best.

## Its pretty safe to state that we are ![cooltext466498248029130](https://github.com/user-attachments/assets/4397b1d3-44f2-4ae9-99c6-7379860bfa73)
