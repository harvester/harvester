# Authentication
Harvester Dashboard supports `local auth` mode for authentication. The default username and password is `admin/password`.

The Harvester login page is shown below:

![auth](./assets/authentication.png)


## App Mode
In `App mode`, which is intended only for development and testing purposes, more authentication modes are configurable using the environment variable `HARVESTER_AUTHENTICATION_MODE`.

The currently supported options are `localUser` (the same as `local auth` mode) and `kubernetesCredentials`.

If the `kubernetesCredentials` authentication option is used, either a kubconfig file or bearer token can provide access to Harvester.