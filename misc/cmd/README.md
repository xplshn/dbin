### Directory explanation:
This directory contains 1 important folder. The rest is for 3rd party projects or backwards compatibility.
The metadata generators under these folder do the following: They fetch the JSON metadata of `pkgforge`, and they modify it to be usable for the current (and past) versions of `dbin`. `dbin` tries to always use the same format as upstream, however, in order to maintain compatibility, the generator is here as middleware, in case something changes or breaks upstream, I can change it here and fix past `dbin` versions.
Not only that, but the metadata generator performs a very key transformation to the metadata, it converts the `pkg` element of the metadata (the name) into the last part of its `download_url`. (meaning, if the binary was named `vi`, and it was part of the busybox family in the repo, it becomes: `busybox/vi`, and so on.)

---
### Unrelated to `dbin`
- The AM-support folder contains a generator used by the AM package manager. (it generates metadata of the AppBundleHUB, in the format which AM requires)
