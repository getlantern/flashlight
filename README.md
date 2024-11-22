# Lantern [![Go Actions Status](https://github.com/getlantern/flashlight/actions/workflows/go.yml/badge.svg)](https://github.com/getlantern/flashlight/actions) [![Coverage Status](https://coveralls.io/repos/github/getlantern/flashlight/badge.svg?t=C4SaZX)](https://coveralls.io/github/getlantern/flashlight)

## Maintainers
@hwh33, @myleshorton, @reflog

This repo contains the core Lantern library as well as the Android and iOS bindings.

The Lantern client application can be found at [lantern-client](https://github.com/getlantern/lantern-client).

## Process for making changes to [config](config)
flashlight is configured with per-proxy configuration loaded from the config-server, and global configuration loaded from S3 at runtime.

The global configuration is generated by [genconfig](genconfig) running as a [CRON job](https://github.com/getlantern/lantern-cloud/blob/main/cmd/update_masquerades/crontab) on lantern-cloud. That job uses the latest version of `genconfig` that is pushed to releases in this repo via CI.

genconfig merges [embeddedconfig/global.yaml.tmpl](embeddedconfig/global.yaml.tmpl) with dynamically verified masquerade hosts to produce the final global config. [embeddedconfig/download_global.sh](embeddedconfig/download_global.sh) pulls in that change and runs anytime we run `go generate`. A [CI Job](https://github.com/getlantern/flashlight/blob/main/.github/workflows/globalconfig.yml) runs `go generate` and automatically commits the results to this repo.

**All clients, including old versions of flashlight, fetch the same global config from S3, so global.yaml.tmpl must remain backwards compatible with old clients.**

If you're simply changing the contents of `global.yaml.tmpl` without any structural changes to the Go files in `config`, you can just change `global.yaml.tmpl` directly and once it's committed to main, it will fairly soon be reflected in S3.

If you're making changes to structure of the configuration, you need to ensure that this stays backwards compatible with old clients. To this end, we keep copies of old versions of [config](config), for example [config_v1](config_v1). When making a structural change to the config, follow these steps:

1. Back up the current version of `config`, for example `cp -R config config_v2`
2. Update the code and tests in `config` as appropriate
3. Make sure the tests in `config_v2`, `config_v1` etc. still work

## Adding new domain fronting mappings

In addition to creating the domain front mappings in Cloudfront and Akamai, you also have to add the appropriate lines to [provider_map.yaml](https://github.com/getlantern/flashlight/blob/main/genconfig/provider_map.yaml) with format: <masked origin domain>: <provider front-facing URL>.
> [!IMPORTANT]
> **Domain mappings must be added for both Cloudfront and Akamai!**

### Adding mapping through `lantern-cloud`
Mappings on Cloudfront and Akamai can be added using the terraform config in [Lantern Cloud](https://github.com/getlantern/lantern-cloud).

### Adding mappings using the web consoles
* **Cloudfront**

    Open up the [Cloudfront Distributions Console](https://us-east-1.console.aws.amazon.com/cloudfront/v3/home?region=us-west-2#/distributions).
    Click 'Create Distribution' and enter the information as follows:
    * **Origin Domain**: Domain Cloudfront will send the request to.
    * **Name**: Use origin domain.
    * **Allowed HTTP methods**: Leave as default if only GET requests are needed, otherwise select the 3rd option to allow POST requests.
    * **Cache key and origin request**: Select the appropiate policies for each.
      - Cache: select optimized or disable if requests should not be cached. 
      - Origin request: select policy based on what user information should be left in the request when sent to the Origin Domain. Can be left at 'None' if user info doesn't need to be forwarded.
    Everything else can be left at the default values unless neccessary. It will take several minutes to deploy after saving. 
    The front-facing URL can be found in the 'Domain name' column from the distributions table. Add this to [provider_map.yaml](https://github.com/getlantern/flashlight/blob/main/genconfig/provider_map.yaml).

* **Akamai**

    Open up the [Akamai Property Manager](https://control.akamai.com/apps/property-manager/#/groups/127281/properties). 
    Click 'New Property' and select 'Dynamic Site Accelerator'. Select 'Property Manager' as the method to set it up. Enter a meaningful property name, select 'latest' as the rule format, and click next.
    **Configuring**

    * **Property Version Information**: 
      > [!IMPORTANT]
      > Security Options must be set to *Standard TLS ready*

    * **Property Hostnames**:
      Click '+Hostnames' -> 'Add Hostname'. Set to `<name>.dsa.akamai.getiantem.org`, where name is a meaningful descriptor (usually the same as the property name). This will be the front-facing URL that is added to [provider_map.yaml](https://github.com/getlantern/flashlight/blob/main/genconfig/provider_map.yaml). Click next and it will generate an Edge Hostname, then submit.
      > [!NOTE]
      > It should now show the hostname in the list and certificate value should be `No certificate (HTTP Only)`.

    * **Property Configuration Settings**: 
      - *Origin Server Hostname*: Domain request will be sent to.
      - *Send True Client IP Header*: Set to no. Leave everything else as the default values.
      - Set the `Origin Server` `Forward Host Header` and `Cache Key Hostname` to `Origin Hostname` if the property is masquerading as the origin.

      Click '+Behavior' -> 'Standard property behavor'.
      - Add content provider code and select 'Site Accelerator - 742552'.
      - Add caching and set appropiate caching option.

      From the tabs on the left go to 'Augment insights' -> 'Traffic reporting' and ensure the content provider code is the same as mentioned above.
      Click 'Save'.

      Go to the activate tab on the top of the page and activate on staging and production. It will take several minutes to activate.

### Testing domain mappings
Domain mappings can be tested using the [ddftool](https://github.com/getlantern/ddftool). You'll want to first use the ddftool to find valid IPs for each provider, then test for the expected response using one of the respective IPs and `https://<provider mapping front-facing domain name>/<some origin domain path for testing>` for the URL.

## Building
In CI, `flashlight` used `GH_TOKEN` for access to private repositories.

You can build an SDK for use by external applications either for Android or for iOS.

### Prerequisites

* [Go 1.19](https://golang.org/dl/) is the minimum supported version of Go
* [GNU Make](https://www.gnu.org/software/make/) if you want to use the Makefile
* Dependencies are managed with Go Modules.
* Force git to use ssh instead of https by running
  `git config --global url."git@github.com:".insteadOf "https://github.com/"`

## A note on iOS and memory usage
The iOS application needs to run as a background process on iOS, meaning that it's severely memory restricted. Because of this, we disable a lot of protocols and extra features using `// go:build !ios` in order to conserve memory.

### Why not use // +build !ios
go-mobile automatically sets the `ios` build tag when building for iOS. In our case, we don't use this because in addition to the iOS app, we also distribute an iOS SDK that's intended for embedding inside of user-interactice apps. This SDK does not have to run in the background and is thus not memory constrained in the same way as our iOS app. Consequently, the sdk can and does include all of the standard lantern protocols and features.

### Architecture

![Overview](https://user-images.githubusercontent.com/1143966/117667942-72c80a80-b173-11eb-8c0d-829f2ccd8cde.png)

## Features

We use "features" to enable/disable different characteristics/techniques in Flashlight, usually through the global config.

See `./config/features.go` for a list of features. Below is a non-extensive description of each feature.

## A Note on Versioning
Until recently, flashlight and the applications that use it used only a single versions number, whatever the application itself was built with. This version number was used for various things:

- Displaying a version number in the UI
- Checking which application features are enabled based on the global configuration
- Telling our server infrastructure (especially config-server) what version of Lantern we're running so that it can assign appropriate proxies based on what that version supports
- Checking whether there's an update available via autoupdate

Because the various applications that use flashlight are in their own repos and built independently of each other, this created a coordination problem. Specifically, since pluggable transport support depends on the code level of flashlight, not of the application itself, we had to either synchronize the version numbering between the different apps, or configure the server-side infrastructure to recognize that depending on the application, different version numbers might actually mean the same flashlight code level.

To rectify this, we now uses two different version numbers.

Library Version - this is the version of the flashlight library and is based on the flashlight version tag.

Application Version - this is like the original Version that's baked in at application build time. It is still used for displaying the version in the UI, checking enabled features, and auto-updates. This version is NOT compiled into the flashlight library but is handled by the applications themselves and passed to flashlight when necessary (for example when checking enabled features).

### When and how to update Library Version
Whenever we release a new version of the flashlight library, we tag it using standard [Go module version numbering], for example `git tag v7.5.3`. Then, we update lantern to use that version of the flashlight library. That's it.

#### What about major version changes
When changing major versions, for example v7 to v8, we need to udpate the package name as usual with Go. That means:

1. Update the `module` directive in `go.mod`
2. Find all imports of `github.com/getlantern/flashlight` and replace with `github.com/getlantern/flashlight/v8`
3. In dependent projects, perform the same search and replace as above
4. Also, dependent projects set embedded flashlight variables in their Makefiles, so make sure to update those paths per the above search and replace too
