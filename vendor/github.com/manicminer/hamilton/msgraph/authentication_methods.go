package msgraph

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/manicminer/hamilton/odata"
)

// AuthenticationMethodsClient performs operations on the Authentications methods endpoint under Identity and Sign-in
type AuthenticationMethodsClient struct {
	BaseClient Client
}

// NewAuthenticationMethodsClient returns a new AuthenticationMethodsClient
func NewAuthenticationMethodsClient(tenantId string) *AuthenticationMethodsClient {
	return &AuthenticationMethodsClient{
		BaseClient: NewClient(VersionBeta, tenantId),
	}
}

// List all authentication methods
func (c *AuthenticationMethodsClient) List(ctx context.Context, userID string, query odata.Query) (*[]AuthenticationMethod, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		DisablePaging:    query.Top > 0,
		OData:            query,
		ValidStatusCodes: []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/users/%s/authentication/methods", userID),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("ApplicationsClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var data struct {
		AuthenticationMethods *[]json.RawMessage `json:"value"`
	}

	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	//The graph API returns a mixture of types, this loop matches up the result to the appropriate model

	var ret []AuthenticationMethod

	if data.AuthenticationMethods == nil {
		return &ret, status, nil
	}

	for _, authMethod := range *data.AuthenticationMethods {
		var o odata.OData
		if err := json.Unmarshal(authMethod, &o); err != nil {
			return nil, status, fmt.Errorf("json.Unmarshall(): %v", err)
		}

		if o.Type == nil {
			continue
		}
		switch *o.Type {
		case odata.TypeFido2AuthenticationMethod:
			var auth Fido2AuthenticationMethod
			if err := json.Unmarshal(authMethod, &auth); err != nil {
				return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
			}
			ret = append(ret, auth)
		case odata.TypeMicrosoftAuthenticatorAuthenticationMethod:
			var auth MicrosoftAuthenticatorAuthenticationMethod
			if err := json.Unmarshal(authMethod, &auth); err != nil {
				return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
			}
			ret = append(ret, auth)
		case odata.TypeWindowsHelloForBusinessAuthenticationMethod:
			var auth WindowsHelloForBusinessAuthenticationMethod
			if err := json.Unmarshal(authMethod, &auth); err != nil {
				return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
			}
			ret = append(ret, auth)
		case odata.TypeTemporaryAccessPassAuthenticationMethod:
			var auth TemporaryAccessPassAuthenticationMethod
			if err := json.Unmarshal(authMethod, &auth); err != nil {
				return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
			}
			ret = append(ret, auth)
		case odata.TypePhoneAuthenticationMethod:
			var auth PhoneAuthenticationMethod
			if err := json.Unmarshal(authMethod, &auth); err != nil {
				return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
			}
			ret = append(ret, auth)
		case odata.TypeEmailAuthenticationMethod:
			var auth EmailAuthenticationMethod
			if err := json.Unmarshal(authMethod, &auth); err != nil {
				return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
			}
			ret = append(ret, auth)
		case odata.TypePasswordAuthenticationMethod:
			var auth PasswordAuthenticationMethod
			if err := json.Unmarshal(authMethod, &auth); err != nil {
				return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
			}
			ret = append(ret, auth)
		}
	}

	return &ret, status, nil
}

func (c *AuthenticationMethodsClient) ListFido2Methods(ctx context.Context, userID string, query odata.Query) (*[]Fido2AuthenticationMethod, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		DisablePaging:    query.Top > 0,
		OData:            query,
		ValidStatusCodes: []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/users/%s/authentication/fido2Methods", userID),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("AuthenticationMethodsClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var data struct {
		Fido2Methods []Fido2AuthenticationMethod `json:"value"`
	}
	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &data.Fido2Methods, status, nil
}

func (c *AuthenticationMethodsClient) GetFido2Method(ctx context.Context, userID, id string, query odata.Query) (*Fido2AuthenticationMethod, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		OData:                  query,
		ValidStatusCodes:       []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/users/%s/authentication/fido2Methods/%s", userID, id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("AuthenticationMethodsClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var fido2Method Fido2AuthenticationMethod
	if err := json.Unmarshal(respBody, &fido2Method); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &fido2Method, status, nil
}

func (c *AuthenticationMethodsClient) DeleteFido2Method(ctx context.Context, userID, id string) (int, error) {
	_, status, _, err := c.BaseClient.Delete(ctx, DeleteHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusNoContent},
		Uri: Uri{
			Entity:      fmt.Sprintf("/users/%s/authentication/fido2Methods/%s", userID, id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return status, fmt.Errorf("AuthenticationMethodsClient.BaseClient.Delete(): %v", err)
	}

	return status, nil
}

func (c *AuthenticationMethodsClient) ListMicrosoftAuthenticatorMethods(ctx context.Context, userID string, query odata.Query) (*[]MicrosoftAuthenticatorAuthenticationMethod, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		DisablePaging:    query.Top > 0,
		OData:            query,
		ValidStatusCodes: []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/users/%s/authentication/microsoftAuthenticatorMethods", userID),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("AuthenticationMethodsClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var data struct {
		MicrosoftAuthenticatorMethods []MicrosoftAuthenticatorAuthenticationMethod `json:"value"`
	}
	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &data.MicrosoftAuthenticatorMethods, status, nil
}

func (c *AuthenticationMethodsClient) GetMicrosoftAuthenticatorMethod(ctx context.Context, userID, id string, query odata.Query) (*MicrosoftAuthenticatorAuthenticationMethod, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		OData:                  query,
		ValidStatusCodes:       []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/users/%s/authentication/microsoftAuthenticatorMethods/%s", userID, id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("AuthenticationMethodsClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var microsoftAuthenticatorMethod MicrosoftAuthenticatorAuthenticationMethod
	if err := json.Unmarshal(respBody, &microsoftAuthenticatorMethod); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &microsoftAuthenticatorMethod, status, nil
}

func (c *AuthenticationMethodsClient) DeleteMicrosoftAuthenticatorMethod(ctx context.Context, userID, id string) (int, error) {
	_, status, _, err := c.BaseClient.Delete(ctx, DeleteHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusNoContent},
		Uri: Uri{
			Entity:      fmt.Sprintf("/users/%s/authentication/microsoftAuthenticatorMethods/%s", userID, id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return status, fmt.Errorf("AuthenticationMethodsClient.BaseClient.Delete(): %v", err)
	}

	return status, nil
}

func (c *AuthenticationMethodsClient) ListWindowsHelloMethods(ctx context.Context, userID string, query odata.Query) (*[]WindowsHelloForBusinessAuthenticationMethod, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		DisablePaging:    query.Top > 0,
		OData:            query,
		ValidStatusCodes: []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/users/%s/authentication/windowsHelloForBusinessMethods", userID),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("AuthenticationMethodsClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var data struct {
		WindowsHelloForBusinessMethods []WindowsHelloForBusinessAuthenticationMethod `json:"value"`
	}
	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &data.WindowsHelloForBusinessMethods, status, nil
}

func (c *AuthenticationMethodsClient) GetWindowsHelloMethod(ctx context.Context, userID, id string, query odata.Query) (*WindowsHelloForBusinessAuthenticationMethod, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		OData:                  query,
		ValidStatusCodes:       []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/users/%s/authentication/windowsHelloForBusinessMethods/%s", userID, id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("AuthenticationMethodsClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var windowsHelloForBusinessMethod WindowsHelloForBusinessAuthenticationMethod
	if err := json.Unmarshal(respBody, &windowsHelloForBusinessMethod); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &windowsHelloForBusinessMethod, status, nil
}

func (c *AuthenticationMethodsClient) DeleteWindowsHelloMethod(ctx context.Context, userID, id string) (int, error) {
	_, status, _, err := c.BaseClient.Delete(ctx, DeleteHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusNoContent},
		Uri: Uri{
			Entity:      fmt.Sprintf("/users/%s/authentication/windowsHelloForBusinessMethods/%s", userID, id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return status, fmt.Errorf("AuthenticationMethodsClient.BaseClient.Delete(): %v", err)
	}

	return status, nil
}

func (c *AuthenticationMethodsClient) ListTemporaryAccessPassMethods(ctx context.Context, userID string, query odata.Query) (*[]TemporaryAccessPassAuthenticationMethod, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		DisablePaging:    query.Top > 0,
		OData:            query,
		ValidStatusCodes: []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/users/%s/authentication/temporaryAccessPassMethods", userID),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("AuthenticationMethodsClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var data struct {
		TempAccessPassMethods []TemporaryAccessPassAuthenticationMethod `json:"value"`
	}
	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &data.TempAccessPassMethods, status, nil
}

func (c *AuthenticationMethodsClient) GetTemporaryAccessPassMethod(ctx context.Context, userID, id string, query odata.Query) (*TemporaryAccessPassAuthenticationMethod, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		OData:                  query,
		ValidStatusCodes:       []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/users/%s/authentication/temporaryAccessPassMethods/%s", userID, id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("AuthenticationMethodsClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var temporaryAccessPassMethod TemporaryAccessPassAuthenticationMethod
	if err := json.Unmarshal(respBody, &temporaryAccessPassMethod); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &temporaryAccessPassMethod, status, nil
}

func (c *AuthenticationMethodsClient) CreateTemporaryAccessPassMethod(ctx context.Context, userID string, accessPass TemporaryAccessPassAuthenticationMethod) (*TemporaryAccessPassAuthenticationMethod, int, error) {
	var status int

	body, err := json.Marshal(accessPass)
	if err != nil {
		return nil, status, fmt.Errorf("json.Marshal(): %v", err)
	}

	resp, status, _, err := c.BaseClient.Post(ctx, PostHttpRequestInput{
		Body:                   body,
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusCreated},
		Uri: Uri{
			Entity:      fmt.Sprintf("/users/%s/authentication/temporaryAccessPassMethods", userID),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("AuthenticationMethodsClient.BaseClient.Post(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var newTempAccessPassAuthMethod TemporaryAccessPassAuthenticationMethod
	if err := json.Unmarshal(respBody, &newTempAccessPassAuthMethod); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &newTempAccessPassAuthMethod, status, nil
}

func (c *AuthenticationMethodsClient) DeleteTemporaryAccessPassMethod(ctx context.Context, userID, id string) (int, error) {
	_, status, _, err := c.BaseClient.Delete(ctx, DeleteHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusNoContent},
		Uri: Uri{
			Entity:      fmt.Sprintf("/users/%s/authentication/temporaryAccessPassMethods/%s", userID, id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return status, fmt.Errorf("AuthenticationMethodsClient.BaseClient.Delete(): %v", err)
	}

	return status, nil
}

func (c *AuthenticationMethodsClient) ListPhoneMethods(ctx context.Context, userID string, query odata.Query) (*[]PhoneAuthenticationMethod, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		DisablePaging:    query.Top > 0,
		OData:            query,
		ValidStatusCodes: []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/users/%s/authentication/phoneMethods", userID),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("AuthenticationMethodsClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var data struct {
		PhoneAuthenticationMethods []PhoneAuthenticationMethod `json:"value"`
	}
	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &data.PhoneAuthenticationMethods, status, nil
}

func (c *AuthenticationMethodsClient) GetPhoneMethod(ctx context.Context, userID, id string, query odata.Query) (*PhoneAuthenticationMethod, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		OData:                  query,
		ValidStatusCodes:       []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/users/%s/authentication/phoneMethods/%s", userID, id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("AuthenticationMethodsClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var phoneMethod PhoneAuthenticationMethod
	if err := json.Unmarshal(respBody, &phoneMethod); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &phoneMethod, status, nil
}

func (c *AuthenticationMethodsClient) CreatePhoneMethod(ctx context.Context, userID string, phone PhoneAuthenticationMethod) (*PhoneAuthenticationMethod, int, error) {
	var status int

	body, err := json.Marshal(phone)
	if err != nil {
		return nil, status, fmt.Errorf("json.Marshal(): %v", err)
	}

	resp, status, _, err := c.BaseClient.Post(ctx, PostHttpRequestInput{
		Body:                   body,
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusCreated},
		Uri: Uri{
			Entity:      fmt.Sprintf("/users/%s/authentication/phoneMethods", userID),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("AuthenticationMethodsClient.BaseClient.Post(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var newPhoneMethod PhoneAuthenticationMethod
	if err := json.Unmarshal(respBody, &newPhoneMethod); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &newPhoneMethod, status, nil
}

func (c *AuthenticationMethodsClient) DeletePhoneMethod(ctx context.Context, userID, id string) (int, error) {
	_, status, _, err := c.BaseClient.Delete(ctx, DeleteHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusNoContent},
		Uri: Uri{
			Entity:      fmt.Sprintf("/users/%s/authentication/phoneMethods/%s", userID, id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return status, fmt.Errorf("AuthenticationMethodsClient.BaseClient.Delete(): %v", err)
	}

	return status, nil
}

func (c *AuthenticationMethodsClient) UpdatePhoneMethod(ctx context.Context, userID string, phone PhoneAuthenticationMethod) (int, error) {
	var status int

	if phone.ID == nil {
		return status, errors.New("AuthenticationMethodsClient.Update(): cannot update phone auth method with nil ID")
	}

	body, err := json.Marshal(phone)
	if err != nil {
		return status, fmt.Errorf("json.Marshal(): %v", err)
	}

	_, status, _, err = c.BaseClient.Put(ctx, PutHttpRequestInput{
		Body:                   body,
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/users/%s/authentication/phoneMethods/%s", userID, *phone.ID),
			HasTenantId: true,
		},
	})
	if err != nil {
		return status, fmt.Errorf("AuthenticationMethodsClient.BaseClient.Put(): %v", err)
	}

	return status, nil
}

func (c *AuthenticationMethodsClient) EnablePhoneSMS(ctx context.Context, userID, id string) (int, error) {
	var status int

	_, status, _, err := c.BaseClient.Post(ctx, PostHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusNoContent},
		Uri: Uri{
			Entity:      fmt.Sprintf("/users/%s/authentication/phoneMethods/%s/enableSmsSignIn", userID, id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return status, fmt.Errorf("AuthenticationMethodsClient.BaseClient.Post(): %v", err)
	}

	return status, nil
}

func (c *AuthenticationMethodsClient) DisablePhoneSMS(ctx context.Context, userID, id string) (int, error) {
	var status int

	_, status, _, err := c.BaseClient.Post(ctx, PostHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusNoContent},
		Uri: Uri{
			Entity:      fmt.Sprintf("/users/%s/authentication/phoneMethods/%s/disableSmsSignIn", userID, id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return status, fmt.Errorf("AuthenticationMethodsClient.BaseClient.Post(): %v", err)
	}

	return status, nil
}

func (c *AuthenticationMethodsClient) ListEmailMethods(ctx context.Context, userID string, query odata.Query) (*[]EmailAuthenticationMethod, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		DisablePaging:    query.Top > 0,
		OData:            query,
		ValidStatusCodes: []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/users/%s/authentication/emailMethods", userID),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("AuthenticationMethodsClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var data struct {
		EmailAuthMethods []EmailAuthenticationMethod `json:"value"`
	}
	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &data.EmailAuthMethods, status, nil
}

func (c *AuthenticationMethodsClient) GetEmailMethod(ctx context.Context, userID, id string, query odata.Query) (*EmailAuthenticationMethod, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		OData:                  query,
		ValidStatusCodes:       []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/users/%s/authentication/emailMethods/%s", userID, id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("AuthenticationMethodsClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var emailMethod EmailAuthenticationMethod
	if err := json.Unmarshal(respBody, &emailMethod); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &emailMethod, status, nil
}

func (c *AuthenticationMethodsClient) UpdateEmailMethod(ctx context.Context, userID string, email EmailAuthenticationMethod) (int, error) {
	var status int

	if email.ID == nil {
		return status, errors.New("AuthenticationMethodsClient.Update(): cannot update phone auth method with nil ID")
	}

	body, err := json.Marshal(email)
	if err != nil {
		return status, fmt.Errorf("json.Marshal(): %v", err)
	}

	_, status, _, err = c.BaseClient.Put(ctx, PutHttpRequestInput{
		Body:                   body,
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/users/%s/authentication/emailMethods/%s", userID, *email.ID),
			HasTenantId: true,
		},
	})
	if err != nil {
		return status, fmt.Errorf("AuthenticationMethodsClient.BaseClient.Put(): %v", err)
	}

	return status, nil
}

func (c *AuthenticationMethodsClient) DeleteEmailMethod(ctx context.Context, userID, id string) (int, error) {
	_, status, _, err := c.BaseClient.Delete(ctx, DeleteHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusNoContent},
		Uri: Uri{
			Entity:      fmt.Sprintf("/users/%s/authentication/emailMethods/%s", userID, id),
			HasTenantId: true,
		},
	})
	if err != nil {
		return status, fmt.Errorf("AuthenticationMethodsClient.BaseClient.Delete(): %v", err)
	}

	return status, nil
}

func (c *AuthenticationMethodsClient) CreateEmailMethod(ctx context.Context, userID string, email EmailAuthenticationMethod) (*EmailAuthenticationMethod, int, error) {
	var status int

	body, err := json.Marshal(email)
	if err != nil {
		return nil, status, fmt.Errorf("json.Marshal(): %v", err)
	}

	resp, status, _, err := c.BaseClient.Post(ctx, PostHttpRequestInput{
		Body:                   body,
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		ValidStatusCodes:       []int{http.StatusCreated},
		Uri: Uri{
			Entity:      fmt.Sprintf("/users/%s/authentication/emailMethods", userID),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("AuthenticationMethodsClient.BaseClient.Post(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var newEmailMethod EmailAuthenticationMethod
	if err := json.Unmarshal(respBody, &newEmailMethod); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &newEmailMethod, status, nil
}

func (c *AuthenticationMethodsClient) ListPasswordMethods(ctx context.Context, userID string, query odata.Query) (*[]PasswordAuthenticationMethod, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		DisablePaging:    query.Top > 0,
		OData:            query,
		ValidStatusCodes: []int{http.StatusOK},
		Uri: Uri{
			Entity:      fmt.Sprintf("/users/%s/authentication/passwordMethods", userID),
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("AuthenticationMethodsClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var data struct {
		PasswordMethods []PasswordAuthenticationMethod `json:"value"`
	}
	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &data.PasswordMethods, status, nil
}

func (c *AuthenticationMethodsClient) GetPasswordMethod(ctx context.Context, userID, id string, query odata.Query) (*PasswordAuthenticationMethod, int, error) {
	resp, status, _, err := c.BaseClient.Get(ctx, GetHttpRequestInput{
		ConsistencyFailureFunc: RetryOn404ConsistencyFailureFunc,
		OData:                  query,
		ValidStatusCodes:       []int{http.StatusOK},
		Uri: Uri{
			HasTenantId: true,
		},
	})
	if err != nil {
		return nil, status, fmt.Errorf("AuthenticationMethodsClient.BaseClient.Get(): %v", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("io.ReadAll(): %v", err)
	}

	var passwordMethod PasswordAuthenticationMethod
	if err := json.Unmarshal(respBody, &passwordMethod); err != nil {
		return nil, status, fmt.Errorf("json.Unmarshal(): %v", err)
	}

	return &passwordMethod, status, nil
}
