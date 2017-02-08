[Website](https://appscode.com) • [Slack](https://slack.appscode.com) • [Forum](https://discuss.appscode.com) • [Twitter](https://twitter.com/AppsCodeHQ)

# voyager
voyager provides controller for [Ingress](#ingress) and [Certificates](#certificate) for Kubernetes developed by [AppsCode](https://appscode.com).

#### Ingress
In here we call it ExtendedIngress.
An extended plugin of Kubernetes [Ingress](https://kubernetes.io/docs/user-guide/ingress/) by AppsCode, to support both L7 and L4 load balancing via a single ingress.
This is built on top of the HAProxy, to support high availability, sticky sessions, name and path-based virtual hosting.
This also support configurable application ports with all the features available in Kubernetes [Ingress](https://kubernetes.io/docs/user-guide/ingress/).

**Feautures**
- HTTP and TCP load balancing,
- TLS Termination,
- Multi-cloud supports,
- Name and Path based virtual hosting,
- Cross namespace routing support,
- URL and Request Header Re-writing,
- Wildcard Name based virtual hosting,
- Persistent sessions, Loadbalancer stats.

#### Certificate
Kubernetes Controller to manage TLS Certificate.

**Feautures**
- Manage Kubernetes TLS secrets backed by Custom Certificate Provider, Users Let's Encrypt by default,
- Manage issued certificates based on Kubernetes ThirdParty Resources,
- Domain validation using ACME dns-01 challenges,
- Support for multiple DNS providers,
- Auto Renew Certificates,
- Use issued Certificates with Ingress to Secure Communications.

## Supported Versions
Kubernetes 1.3+

## User Guide
To deploy voyager in Kubernetes follow this [guide](docs/user-guide/README.md). In short this contains those two steps

1. Create `ingress.appscode.com` and `certificate.appscode.com` Third Party Resource
2. Deploy voyager to kubernetes.


## Developer Guide
Want to learn whats happening under the hood, read [this](docs/developer-guide/README.md).

## Contribution
If you're interested in being a contributor, read [this](docs/contribution/README.md).

## Building voyager
Read [Build Instructions](docs/developer-guide/build.md) to build voyager.

## Support
If you have any questions, you can reach out to us.

* [Slack](https://slack.appscode.com)
* [Forum](https://discuss.appscode.com)
* [Twitter](https://twitter.com/AppsCodeHQ)
* [Website](https://appscode.com)