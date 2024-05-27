package odata

const (
	ErrorAddedObjectReferencesAlreadyExist      = "One or more added object references already exist"
	ErrorCannotDeleteOrUpdateEnabledEntitlement = "Permission (scope or role) cannot be deleted or updated unless disabled first"
	ErrorConflictingObjectPresentInDirectory    = "A conflicting object with one or more of the specified property values is present in the directory"
	ErrorNotValidReferenceUpdate                = "Not a valid reference update"
	ErrorResourceDoesNotExist                   = "Resource '.+' does not exist or one of its queried reference-property objects are not present"
	ErrorRemovedObjectReferencesDoNotExist      = "One or more removed object references do not exist"
	ErrorServicePrincipalAppInOtherTenant       = "When using this permission, the backing application of the service principal being created must in the local tenant"
	ErrorServicePrincipalInvalidAppId           = "The appId '.+' of the service principal does not reference a valid application object"
	ErrorUnknownUnsupportedQuery                = "UnknownError: Unsupported Query"
)
