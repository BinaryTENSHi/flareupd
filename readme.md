# flareupd

A simple daemon for updating a dynamic dns entry on Cloudflare.  
Both IPv4 and IPv6 supported.

## Usage

1. Download the latest release from Github [here](https://github.com/BinaryTENSHi/flareupd/releases)
2. Export the required environment variables
3. Launch the application

Alternatively, you can use the example systemd service to launch the binary as a service.

The application will automatically try and fetch the IPv4 and IPv6 address.
Should one of them fail, an appropriate message will be printed and the respective type will be disabled for further updating.

## Required environment variables

| Variable     | Description                                                    |
|:-------------|:---------------------------------------------------------------|
| CF_API_KEY   | Your cloudflare api key                                        |
| CF_API_EMAIL | Your cloudflare email                                          |
| CF_ZONE_NAME | Your cloudflare zone name                                      |
| REFRESH      | Time in seconds to fetch the external ip and update the record |
| ENTRY        | The name of the dns record to update                           |

_CF_ZONE_NAME=binary.network_ and _ENTRY=flareupd_ will update the domain _flareupd.binary.network_.

## Optional environment variables

| Variable     | Description                              | Default              |
|:-------------|:-----------------------------------------|:---------------------|
| IP4_INFO_URL | URL to use for fetching the IPv4 address | https://v4.ident.me/ |
| IP6_INFO_URL | URL to use for fetching the IPv6 address | https://v6.ident.me/ |

These urls must only return the IP address in no special format.
