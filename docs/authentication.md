# Authentication
Harvester Dashboard supports `local auth` mode, the default login username and password is `admin/password`,

![](./assets/authentication.png)


### App Mode
on `App mode`, users are allowing to configure different authentication mode using env `HARVESTER_AUTHENTICATION_MODE`, the current supported options are `localUser` and `kubernetesCredentials`:

- [Kubeconfig](#kubeconfig) file that can be used on Dashboard login view.
- [Bearer Token](#bearer-token)

### Kubeconfig

`Kubeconfig` authentication method does not support external identity providers or basic authentications.

### Bearer Token

It is recommended to get familiar with [Kubernetes authentication](https://kubernetes.io/docs/reference/access-authn-authz/authentication/) documentation first to find out how to get token, that can be used to login. In example every Service Account has a Secret with valid Bearer Token that can be used to login to Dashboard.

Recommended lecture to find out how to create Service Account and grant it privileges:

* [Service Account Tokens](https://kubernetes.io/docs/reference/access-authn-authz/authentication/#service-account-tokens)
* [Role and ClusterRole](https://kubernetes.io/docs/reference/access-authn-authz/rbac/#role-and-clusterrole)
* [Service Account Permissions](https://kubernetes.io/docs/reference/access-authn-authz/rbac/#service-account-permissions)

To create sample user and to get its token, see [Creating sample user](https://github.com/kubernetes/dashboard/blob/master/docs/user/access-control/creating-sample-user.md) guide.
