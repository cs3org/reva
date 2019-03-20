package ocdavsvc

import (
	"encoding/xml"
	"io"
	"net/http"
)

func (s *svc) doReport(w http.ResponseWriter, r *http.Request) {

	ctx := r.Context()
	//fn := r.URL.Path

	rep, status, err := readReport(r.Body)
	if err != nil {
		logger.Error(ctx, err)
		w.WriteHeader(status)
		return
	}
	if rep.SearchFiles != nil {
		s.doSearchFiles(w, r, rep.SearchFiles)
		return
	}

	w.WriteHeader(http.StatusNotImplemented)
}

func (s *svc) doSearchFiles(w http.ResponseWriter, r *http.Request, sf *reportSearchFiles) {
	ctx := r.Context()
	_, err := s.getClient()
	if err != nil {
		logger.Error(ctx, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNotImplemented)
}

type report struct {
	SearchFiles *reportSearchFiles
	// FilterFiles TODO add this for tag based search
}
type reportSearchFiles struct {
	XMLName xml.Name                `xml:"search-files"`
	Lang    string                  `xml:"xml:lang,attr,omitempty"`
	Prop    propfindProps           `xml:"DAV: prop"`
	Search  reportSearchFilesSearch `xml:"search"`
}
type reportSearchFilesSearch struct {
	Pattern string `xml:"search"`
	Limit   int    `xml:"limit"`
	Offset  int    `xml:"offset"`
}

func readReport(r io.Reader) (rep *report, status int, err error) {
	decoder := xml.NewDecoder(r)
	rep = &report{}
	for {
		t, err := decoder.Token()
		if err == io.EOF {
			// io.EOF is a successful end
			return rep, 0, nil
		}
		if err != nil {
			return nil, http.StatusBadRequest, err
		}

		switch v := t.(type) {

		case xml.StartElement:
			if v.Name.Local == "search-files" {
				var repSF reportSearchFiles
				err = decoder.DecodeElement(&repSF, &v)
				if err != nil {
					return nil, http.StatusBadRequest, err
				}
				rep.SearchFiles = &repSF
			}
		}
	}
}
