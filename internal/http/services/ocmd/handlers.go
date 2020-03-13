// Copyright 2018-2020 CERN
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// In applying this license, CERN does not waive the privileges and immunities
// granted to it by virtue of its status as an Intergovernmental Organization
// or submit itself to any jurisdiction.

package ocmd

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog"
)

func (s *svc) trustedDomainCheck(logger *zerolog.Logger, providerAuthorizer providerAuthorizer, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		remoteAddress := r.RemoteAddr // ip:port
		clientIP := strings.Split(remoteAddress, ":")[0]
		domains, err := net.LookupAddr(clientIP)
		if err != nil {
			logger.Err(err).Msg("error getting domain for IP")
			w.WriteHeader(http.StatusForbidden)
			ae := newAPIError(apiErrorUntrustedService)
			w.Write(ae.JSON())
			return
		}
		logger.Debug().Str("ip", clientIP).Str("remote-address", remoteAddress).Str("domains", fmt.Sprintf("%+v", domains)).Msg("ip resolution")

		allowedDomain := ""
		for _, domain := range domains {
			if err := providerAuthorizer.IsProviderAllowed(ctx, domain); err == nil {
				allowedDomain = domain
				break
			}
			logger.Debug().Str("domain", domain).Msg("domain not allowed")
		}

		if allowedDomain == "" {
			logger.Err(errors.New("provider is not allowed to use the API")).Str("remote-address", remoteAddress)
			w.WriteHeader(http.StatusForbidden)
			ae := newAPIError(apiErrorUntrustedService)
			w.Write(ae.JSON())
			return
		}

		logger.Debug().Str("remote-address", remoteAddress).Str("domain", allowedDomain).Msg("provider is allowd to access the API")
		mux.Vars(r)["domain"] = allowedDomain
		h.ServeHTTP(w, r)
	})
}

func (s *svc) propagateInternalShare(logger *zerolog.Logger, sm shareManager, pa providerAuthorizer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		shareID := r.FormValue("shareID")

		share, err := sm.GetInternalShare(ctx, shareID)
		if err != nil {
			logger.Err(err).Msg("Error getting share")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			ae := newAPIError(apiErrorInvalidParameter).WithMessage("Could not retrieve share ID")
			w.Write(ae.JSON())
			return
		}

		domain := getDomainFromMail(share.ShareWith)
		providerInfo, err := pa.GetProviderInfoByDomain(ctx, domain)
		if err != nil {
			logger.Err(err).Str("input-domain", domain).Msg("error getting provider info")

		}
		logger.Info().Str("domain", providerInfo.Domain).Str("ocm-endpoint", providerInfo.APIEndPoint).Msg("provider info")
		reqBody := bytes.NewReader(share.JSON())
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		client := http.Client{Transport: tr}
		req, err := http.NewRequest("POST", providerInfo.APIEndPoint+"/shares", reqBody)
		if err != nil {
			logger.Err(err).Msg("error preparing outgoing request for new share")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			logger.Err(err).Msg("Error trying to post share to provider endpoint")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			ae := newAPIError(apiErrorProviderError).WithMessage("Remote OCM endpoint not reachable")
			w.Write(ae.JSON())
			return
		}
		if resp.StatusCode != http.StatusCreated {
			body, _ := ioutil.ReadAll(resp.Body)
			logger.Err(errors.New("wrong status code from ocm endpoint")).Int("expected", http.StatusCreated).Int("got", resp.StatusCode).Str("body", string(body))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			ae := newAPIError(apiErrorProviderError).WithMessage("Wrong status code from OCM endpoint")
			w.Write(ae.JSON())
			return

		}

		logger.Debug().Msg("consumer share has been created on the remote OCM instance")

		w.WriteHeader(http.StatusCreated)
		w.Write(share.JSON())
	})

}

func (s *svc) notImplemented(logger *zerolog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotImplemented)
		apiErr := newAPIError(apiErrorUnimplemented)
		w.Write(apiErr.JSON())
	})

}

func (s *svc) getOCMInfo(logger *zerolog.Logger, host string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		info := apiInfo{
			Enabled:    true,
			APIVersion: "1.0-proposal1",
			EndPoint:   fmt.Sprintf("https://%s/cernbox/ocm", host),
			ResourceTypes: []resourceTypes{resourceTypes{
				Name:       "file",
				ShareTypes: []string{"user"},
				Protocols: resourceTypesProtocols{
					Webdav: "/cernbox/ocm_webdav",
				},
			}},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(info.JSON())
		// w.Write([]byte("{\"enabled\":true,\"apiVersion\":\"1.0-proposal1\",\"endPoint\":\"https:\\/\\/cernbox.up2u.cern.ch\\/cernbox\\/ocm\",\"resourceTypes\":[{\"name\":\"file\",\"shareTypes\":[\"user\",\"group\"],\"protocols\":{\"webdav\":\"\\/cernbox\\/ocm_webdav\"}}]}"))

	})
}

func (s *svc) addProvider(logger *zerolog.Logger, pa providerAuthorizer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		if err := r.ParseForm(); err != nil {
			logger.Err(err).Msg("Error parsing request")
			return
		}

		domain := r.FormValue("domain")

		//TODO error if empty

		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		client := &http.Client{Transport: tr}

		url := fmt.Sprintf("%s/ocm-provider/", domain)
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {

			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		res, err := client.Do(req)
		if err != nil {
			logger.Err(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if res.StatusCode != http.StatusOK {
			logger.Err(errors.New("Error getting provider info")).Int("status", res.StatusCode)
			w.WriteHeader(res.StatusCode)
			return
		}

		body, err := ioutil.ReadAll(res.Body)
		if err != nil {
			logger.Err(err).Msg("Error reading body")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		apiInfo := &apiInfo{}
		err = json.Unmarshal(body, apiInfo)
		if err != nil {
			logger.Err(err).Msg("Error parsing provider info")
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		domainWithoutProtocol := strings.Replace(domain, "http://", "", 1)
		domainWithoutProtocol = strings.Replace(domainWithoutProtocol, "https://", "", 1)

		internalProvider := &providerInfo{
			Domain:         domainWithoutProtocol,
			APIVersion:     apiInfo.APIVersion,
			APIEndPoint:    apiInfo.EndPoint,
			WebdavEndPoint: domain + apiInfo.ResourceTypes[0].Protocols.Webdav, //TODO check this instead of hardcode + support for multiple webdav
		}

		pa.AddProvider(ctx, internalProvider)
		w.WriteHeader(http.StatusOK)

	})
}

func (s *svc) addShare(logger *zerolog.Logger, sm shareManager, pa providerAuthorizer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		ctx := r.Context()

		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			logger.Err(err).Msg("error reading body of the request")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		// logger.Info().Str("BODY", string(body)).Msg("POST >>>")

		share := &share{}
		err = json.Unmarshal(body, share)
		// if err != nil {
		// 	logger.Err(err).Msg("error unmarshaling body into share, trying again")

		// 	// OC send providerId as int....
		// 	type share struct {
		// 		ShareWith         string            `json:"shareWith"`
		// 		Name              string            `json:"name"`
		// 		Description       string            `json:"description"`
		// 		ProviderID        int               `json:"providerId"`
		// 		Owner             string            `json:"owner"`
		// 		Sender            string            `json:"sender"`
		// 		OwnerDisplayName  string            `json:"ownerDisplayName"`
		// 		SenderDisplayName string            `json:"senderDisplayName"`
		// 		ShareType         string            `json:"shareType"`
		// 		ResourceType      string            `json:"resourceType"`
		// 		Protocol          *protocolInfo `json:"protocol"`

		// 		ID        string `json:"id,omitempty"`
		// 		CreatedAt string `json:"createdAt,omitempty"`
		// 	}
		// 	share2Obj := &share2{}
		// 	err = json.Unmarshal(body, share2Obj)

		// 	if err != nil {
		// 		logger.Err(err).Msg("error unmarshaling body into share")
		// 		w.Header().Set("Content-Type", "application/json")
		// 		w.WriteHeader(http.StatusBadRequest)
		// 		ae := newAPIError(APIErrorInvalidParameter).WithMessage("body is not json")
		// 		w.Write(ae.JSON())
		// 		return
		// 	}

		// 	share = &share{
		// 		ShareWith:         share2Obj.ShareWith,
		// 		Name:              share2Obj.Name,
		// 		Description:       share2Obj.Description,
		// 		ProviderID:        strconv.Itoa(share2Obj.ProviderID),
		// 		Owner:             share2Obj.Owner,
		// 		Sender:            share2Obj.Sender,
		// 		OwnerDisplayName:  share2Obj.OwnerDisplayName,
		// 		SenderDisplayName: share2Obj.SenderDisplayName,
		// 		ShareType:         share2Obj.ShareType,
		// 		ResourceType:      share2Obj.ResourceType,
		// 		Protocol:          share2Obj.Protocol,
		// 		ID:                share2Obj.ID,
		// 		CreatedAt:         share2Obj.CreatedAt,
		// 	}

		// }

		if err != nil {
			logger.Err(err).Msg("error unmarshaling body into share")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			ae := newAPIError(apiErrorInvalidParameter).WithMessage("body is not json")
			w.Write(ae.JSON())
			return
		}

		logger.Debug().Str("share", fmt.Sprintf("%+v", share)).Msg("received share from client")

		// OC sends this with http..... (TODO: find better way of cleaning this)
		share.Owner = strings.Replace(share.Owner, "http://", "", 1)
		share.Owner = strings.Replace(share.Owner, "https://", "", 1)
		share.Sender = strings.Replace(share.Sender, "http://", "", 1)
		share.Sender = strings.Replace(share.Sender, "https://", "", 1)
		share.Owner = strings.Replace(share.Owner, "http:\\/\\/", "", 1)
		share.Owner = strings.Replace(share.Owner, "https:\\/\\/", "", 1)
		share.Sender = strings.Replace(share.Sender, "http:\\/\\/", "", 1)
		share.Sender = strings.Replace(share.Sender, "https:\\/\\/", "", 1)

		owner := strings.Split(share.Owner, "@")

		if len(owner) != 2 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			ae := newAPIError(apiErrorInvalidParameter).WithMessage("owner must contain domain")
			w.Write(ae.JSON())
			return
		}

		domain := owner[1]

		if err = pa.IsProviderAllowed(ctx, domain); err != nil {
			logger.Debug().Str("owner", share.Owner).Str("domain", domain).Msg("Unauthorized owner of share")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			ae := newAPIError(apiErrorInvalidParameter).WithMessage("owner domain is not allowed")
			w.Write(ae.JSON())
			return
		}

		shareWith := strings.Split(share.ShareWith, "@")
		//TODO check user exists and domain valid, possibly get username if email was given !!!

		newShare, err := sm.NewShare(ctx, share, domain, shareWith[0])
		if err != nil {
			logger.Err(err).Msg("error creating share")
			if ae, ok := err.(*apiError); ok {
				if ae.Code == apiErrorInvalidParameter {
					logger.Err(err).Msg("invalid parameters provided for creating share")
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusBadRequest)
					w.Write(ae.JSON())
					return

				}
			}
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusCreated)
		w.Write(newShare.JSON())

	})
}

func (s *svc) proxyWebdav(logger *zerolog.Logger, sm shareManager, pa providerAuthorizer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		// user, _, ok := r.BasicAuth()
		// if !ok {
		// 	logger.Debug("No auth")
		// 	w.WriteHeader(http.StatusForbidden)
		// 	return
		// }
		// With oauth we receive a header with username
		user := r.Header.Get("Remote-User")

		requestPath := mux.Vars(r)["path"]
		logger.Info().Str("user", user).Str("path", requestPath).Msg("WEBDAV PROXY")

		logRequest(logger, r)

		pathElements := strings.FieldsFunc(requestPath, getSplitFunc('/'))

		if len(pathElements) == 0 {

			if r.Method == "OPTIONS" {

				w.Header().Set("dav", "1,2")
				w.Header().Set("allow", "OPTIONS,PROPFIND")

			} else if r.Method == "PROPFIND" {

				bodyb, _ := ioutil.ReadAll(r.Body)
				body := string(bodyb)

				shares, _ := sm.GetShares(ctx, user)

				toReturn := "<?xml version=\"1.0\" encoding=\"utf-8\"?>" +
					"<d:multistatus xmlns:d=\"DAV:\" xmlns:oc=\"http://owncloud.org/ns\">"

				toReturn = toReturn + "<d:response>" +
					"<d:href>/cernbox/desktop/remote.php/webdav/ocm/</d:href>" +
					"<d:propstat>" +
					"<d:status>HTTP/1.1 200 OK</d:status>" +
					"<d:prop>"

				if strings.Contains(body, "getlastmodified") {
					toReturn = toReturn + fmt.Sprintf("<d:getlastmodified>%s</d:getlastmodified>", time.Now().Format(time.RFC1123))
				}

				if strings.Contains(body, "creationdate") {
					toReturn = toReturn + "<d:creationdate>2018-12-18T00:00:00Z</d:creationdate>"
				}

				if strings.Contains(body, "getetag") {
					toReturn = toReturn + fmt.Sprintf("<d:getetag>&quot;60:%s&quot;</d:getetag>", string(rand.Intn(100)))
				}

				if strings.Contains(body, "oc:id") {
					toReturn = toReturn + "<oc:id>0</oc:id>"
				}

				if strings.Contains(body, "size") {
					toReturn = toReturn + "<oc:size>2</oc:size>"
				}

				if strings.Contains(body, "permissions") {
					toReturn = toReturn + "<oc:permissions>RWCKNVD</oc:permissions>"
				}

				if strings.Contains(body, "displayname") {
					toReturn = toReturn + "<d:displayname>ocm</d:displayname>"
				}

				toReturn = toReturn + "<d:resourcetype>" +
					"<d:collection/>" +
					"</d:resourcetype>" +
					"</d:prop>" +
					"</d:propstat>" +
					"<d:propstat>" +
					"<d:status>HTTP/1.1 404 Not Found</d:status>" +
					"<d:prop/>" +
					"</d:propstat>" +
					"</d:response>"

				type shareXML struct {
					Name string
					XML  string
				}

				sharesXML := []*shareXML{}

				for i := 0; i < len(shares); i++ {

					name := shares[i].Name
					name = strings.Replace(name, "/", "", 1)
					name = name + " (id-" + shares[i].ID + ")"
					nameURL, _ := url.Parse(name)
					name = nameURL.EscapedPath()

					xml := fmt.Sprintf("<d:response>"+
						"<d:href>/cernbox/desktop/remote.php/webdav/ocm/%s/</d:href>"+
						"<d:propstat>"+
						"<d:status>HTTP/1.1 200 OK</d:status>"+
						"<d:prop>", name)

					if strings.Contains(body, "getlastmodified") {
						xml = xml + fmt.Sprintf("<d:getlastmodified>%s</d:getlastmodified>", time.Now().Format(time.RFC1123))
					}

					if strings.Contains(body, "creationdate") {
						xml = xml + fmt.Sprintf("<d:creationdate>%s</d:creationdate>", shares[i].CreatedAt)
					}

					if strings.Contains(body, "getetag") {
						xml = xml + fmt.Sprintf("<d:getetag>&quot;60:%s:%s&quot;</d:getetag>", shares[i].ID, string(rand.Intn(100)))
					}

					if strings.Contains(body, "oc:id") {
						xml = xml + fmt.Sprintf("<oc:id>%s</oc:id>", shares[i].ID)
					}

					if strings.Contains(body, "size") {
						xml = xml + "<oc:size>2</oc:size>"
					}

					if strings.Contains(body, "permissions") {
						xml = xml + "<oc:permissions>RWCKNVD</oc:permissions>"
					}

					if strings.Contains(body, "displayname") {
						xml = xml + fmt.Sprintf("<d:displayname>%s</d:displayname>", name)
					}

					xml = xml + "<d:resourcetype>" +
						"<d:collection/>" +
						"</d:resourcetype>" +
						"</d:prop>" +
						"</d:propstat>" +
						"<d:propstat>" +
						"<d:status>HTTP/1.1 404 Not Found</d:status>" +
						"<d:prop/>" +
						"</d:propstat>" +
						"</d:response>"

					sharesXML = append(sharesXML, &shareXML{
						Name: name,
						XML:  xml,
					})
				}

				sort.Slice(sharesXML, func(i, j int) bool {
					return sharesXML[i].Name < sharesXML[j].Name
				})

				for i := 0; i < len(sharesXML); i++ {
					toReturn = toReturn + sharesXML[i].XML
				}

				toReturn = toReturn + "</d:multistatus>"
				bToReturn := []byte(toReturn)
				w.Header().Set("Content-Type", "application/xml; charset=utf-8")
				w.Header().Set("Content-Length", strconv.Itoa(len(bToReturn)))
				w.WriteHeader(http.StatusMultiStatus)
				w.Write(bToReturn)

			} else {
				w.WriteHeader(http.StatusForbidden)
			}

		} else {

			currentShareName := pathElements[0]

			shareIDElements := strings.Split(currentShareName, "(id-")
			shareID := shareIDElements[len(shareIDElements)-1]
			shareID = strings.Replace(shareID, ")", "", 1)

			share, err := sm.GetExternalShare(ctx, user, shareID)

			if err != nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}

			providerElements := strings.Split(share.Owner, "@")

			provider, err := pa.GetProviderInfoByDomain(ctx, providerElements[1])

			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			replaceLocalPath := fmt.Sprintf("/cernbox/desktop/remote.php/webdav/ocm/%s", currentShareName)
			replaceLocalPathURL, _ := url.Parse(replaceLocalPath)
			replaceLocalPathURLEscaped := replaceLocalPathURL.EscapedPath()

			remotePath := strings.Join(pathElements[1:], "/")
			remoteURL, _ := url.Parse(strings.Replace(path.Join(provider.WebdavEndPoint, remotePath), "https:/", "https://", 1))

			replaceRemotePathURL, _ := url.Parse(provider.WebdavEndPoint)
			replaceRemotePath := replaceRemotePathURL.Path
			replaceRemotePathElems := strings.Split(replaceRemotePath, "/")
			replaceRemotePath = "/" + strings.Join(replaceRemotePathElems[1:], "/")

			replaceRemotePathURL, _ = url.Parse(replaceRemotePath)
			replaceRemotePathURLEscaped := replaceRemotePathURL.EscapedPath()

			logger.Info().Str("remotePath", remotePath).Str("remoteURL", remoteURL.String()).Str("replaceRemotePath", replaceRemotePathURLEscaped)

			http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
			proxy := httputil.NewSingleHostReverseProxy(remoteURL)

			// CHECK OTHER METHODS
			if r.Method == "PROPFIND" {
				proxy.ModifyResponse = rewriteHref(replaceRemotePathURLEscaped, replaceLocalPathURLEscaped)
			} else if r.Method == "MOVE" {
				destination := r.Header.Get("destination")
				destinationElems := strings.Split(destination, replaceLocalPathURLEscaped)
				destinationURL, _ := url.Parse(strings.Replace(path.Join(provider.WebdavEndPoint, destinationElems[1]), "https:/", "https://", 1))
				// logger.Info("INFO", zap.String("DESTINATION", destinationURL.String()))
				r.Header.Set("destination", destinationURL.String())
			}

			r.URL, _ = url.Parse("")
			r.Host = remoteURL.Host
			r.SetBasicAuth(share.Protocol.Options.SharedSecret, share.Protocol.Options.SharedSecret)

			proxy.ServeHTTP(w, r)

		}

	})
}

func getDomainFromMail(mail string) string {
	tokens := strings.Split(mail, "@")
	return tokens[len(tokens)-1]

}

func getSplitFunc(separator rune) func(rune) bool {
	return func(c rune) bool {
		return c == separator
	}
}

func rewriteHref(oldPath, newPath string) func(resp *http.Response) (err error) {
	return func(resp *http.Response) (err error) {

		if resp.StatusCode == 401 || (resp.StatusCode >= 300 && resp.StatusCode < 400) {
			// If the sharer revoked the share, we shouldn't make our users login again
			// If we got a redirect, we shouldn't follow it for security reasons
			resp.StatusCode = 404
			return nil
		}

		contentType := resp.Header.Get("Content-type")
		if !strings.Contains(contentType, "application/xml") {
			return nil
		}

		b, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		err = resp.Body.Close()
		if err != nil {
			return err
		}
		b = bytes.Replace(b, []byte(oldPath), []byte(newPath), -1) // replace html
		body := ioutil.NopCloser(bytes.NewReader(b))
		resp.Body = body
		resp.ContentLength = int64(len(b))
		resp.Header.Set("Content-Length", strconv.Itoa(len(b)))
		return nil
	}
}

func logRequest(logger *zerolog.Logger, r *http.Request) {

	// Create return string
	var request []string
	// Add the request string
	myURL := fmt.Sprintf("%v %v %v", r.Method, r.URL, r.Proto)
	request = append(request, myURL)
	// Add the host
	request = append(request, fmt.Sprintf("Host: %v", r.Host))
	// Loop through headers
	for name, headers := range r.Header {
		name = strings.ToLower(name)
		for _, h := range headers {
			request = append(request, fmt.Sprintf("%v: %v", name, h))
		}
	}

	// If this is a POST, add post data
	if r.Method == "POST" {
		r.ParseForm()
		request = append(request, "\n")
		request = append(request, r.Form.Encode())
	}

	logger.Info().Str(r.Method, strings.Join(request, " +++ ")).Msg("REQUEST")
}
