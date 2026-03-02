package server

import (
	"encoding/json"
	"io"
	"net/http"
)

func (s *Server) registerOpenMeterRoutes(mux *http.ServeMux) {
	// Meters
	mux.HandleFunc("GET /api/openmeter/meters", s.omProxy(func() (json.RawMessage, error) {
		return s.om.ListMeters()
	}))
	mux.HandleFunc("POST /api/openmeter/meters", s.omBodyProxy(func(body json.RawMessage) (json.RawMessage, error) {
		return s.om.CreateMeter(body)
	}))
	mux.HandleFunc("GET /api/openmeter/meters/{id}", s.omPathProxy(func(r *http.Request) (json.RawMessage, error) {
		return s.om.GetMeter(r.PathValue("id"))
	}))
	mux.HandleFunc("PUT /api/openmeter/meters/{id}", s.omPathBodyProxy(func(r *http.Request, body json.RawMessage) (json.RawMessage, error) {
		return s.om.UpdateMeter(r.PathValue("id"), body)
	}))
	mux.HandleFunc("DELETE /api/openmeter/meters/{id}", s.omDeleteProxy(func(r *http.Request) error {
		return s.om.DeleteMeter(r.PathValue("id"))
	}))
	mux.HandleFunc("POST /api/openmeter/meters/{id}/query", s.omPathBodyProxy(func(r *http.Request, body json.RawMessage) (json.RawMessage, error) {
		return s.om.QueryMeter(r.PathValue("id"), body)
	}))
	mux.HandleFunc("GET /api/openmeter/meters/{id}/group-by/{key}/values", s.omPathProxy(func(r *http.Request) (json.RawMessage, error) {
		return s.om.ListMeterGroupByValues(r.PathValue("id"), r.PathValue("key"), r.URL.RawQuery)
	}))

	// Features
	mux.HandleFunc("GET /api/openmeter/features", s.omProxy(func() (json.RawMessage, error) {
		return s.om.ListFeatures(false)
	}))
	mux.HandleFunc("POST /api/openmeter/features", s.omBodyProxy(func(body json.RawMessage) (json.RawMessage, error) {
		return s.om.CreateFeature(body)
	}))
	mux.HandleFunc("GET /api/openmeter/features/{id}", s.omPathProxy(func(r *http.Request) (json.RawMessage, error) {
		return s.om.GetFeature(r.PathValue("id"))
	}))
	mux.HandleFunc("DELETE /api/openmeter/features/{id}", s.omDeleteProxy(func(r *http.Request) error {
		return s.om.DeleteFeature(r.PathValue("id"))
	}))

	// Customers
	mux.HandleFunc("GET /api/openmeter/customers", s.omProxy(func() (json.RawMessage, error) {
		return s.om.ListCustomers()
	}))
	mux.HandleFunc("GET /api/openmeter/customers/{id}", s.omPathProxy(func(r *http.Request) (json.RawMessage, error) {
		return s.om.GetCustomer(r.PathValue("id"))
	}))
	mux.HandleFunc("PUT /api/openmeter/customers/{id}", s.omPathBodyProxy(func(r *http.Request, body json.RawMessage) (json.RawMessage, error) {
		return s.om.UpdateCustomer(r.PathValue("id"), body)
	}))
	mux.HandleFunc("DELETE /api/openmeter/customers/{id}", s.omDeleteProxy(func(r *http.Request) error {
		return s.om.DeleteCustomer(r.PathValue("id"))
	}))
	mux.HandleFunc("GET /api/openmeter/customers/{id}/access", s.omPathProxy(func(r *http.Request) (json.RawMessage, error) {
		return s.om.GetCustomerAccess(r.PathValue("id"))
	}))
	mux.HandleFunc("GET /api/openmeter/customers/{id}/apps", s.omPathProxy(func(r *http.Request) (json.RawMessage, error) {
		return s.om.ListCustomerApps(r.PathValue("id"))
	}))
	mux.HandleFunc("GET /api/openmeter/customers/{id}/entitlements", s.omPathProxy(func(r *http.Request) (json.RawMessage, error) {
		return s.om.ListCustomerEntitlements(r.PathValue("id"))
	}))
	mux.HandleFunc("POST /api/openmeter/customers/{id}/entitlements", s.omPathBodyProxy(func(r *http.Request, body json.RawMessage) (json.RawMessage, error) {
		return s.om.CreateCustomerEntitlement(r.PathValue("id"), body)
	}))
	mux.HandleFunc("DELETE /api/openmeter/customers/{custId}/entitlements/{entId}", s.omDeleteProxy(func(r *http.Request) error {
		return s.om.DeleteCustomerEntitlement(r.PathValue("custId"), r.PathValue("entId"))
	}))
	mux.HandleFunc("GET /api/openmeter/customers/{custId}/entitlements/{entId}/value", s.omPathProxy(func(r *http.Request) (json.RawMessage, error) {
		return s.om.GetEntitlementValue(r.PathValue("custId"), r.PathValue("entId"))
	}))
	mux.HandleFunc("GET /api/openmeter/customers/{custId}/entitlements/{entId}/grants", s.omPathProxy(func(r *http.Request) (json.RawMessage, error) {
		return s.om.ListEntitlementGrants(r.PathValue("custId"), r.PathValue("entId"))
	}))
	mux.HandleFunc("POST /api/openmeter/customers/{custId}/entitlements/{entId}/grants", s.omPathBodyProxy(func(r *http.Request, body json.RawMessage) (json.RawMessage, error) {
		return s.om.CreateEntitlementGrant(r.PathValue("custId"), r.PathValue("entId"), body)
	}))
	mux.HandleFunc("POST /api/openmeter/customers/{custId}/entitlements/{entId}/reset", s.omPathBodyProxy(func(r *http.Request, body json.RawMessage) (json.RawMessage, error) {
		return s.om.ResetEntitlement(r.PathValue("custId"), r.PathValue("entId"), body)
	}))

	// Events
	mux.HandleFunc("GET /api/openmeter/events", s.omPathProxy(func(r *http.Request) (json.RawMessage, error) {
		return s.om.ListEvents(r.URL.RawQuery)
	}))
	mux.HandleFunc("POST /api/openmeter/events", s.omIngestProxy())
}

// Proxy helpers to reduce boilerplate

func (s *Server) omProxy(fn func() (json.RawMessage, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		data, err := fn()
		if err != nil {
			writeErr(w, http.StatusBadGateway, err.Error())
			return
		}
		writeRawJSON(w, http.StatusOK, data)
	}
}

func (s *Server) omPathProxy(fn func(*http.Request) (json.RawMessage, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := fn(r)
		if err != nil {
			writeErr(w, http.StatusBadGateway, err.Error())
			return
		}
		writeRawJSON(w, http.StatusOK, data)
	}
}

func (s *Server) omBodyProxy(fn func(json.RawMessage) (json.RawMessage, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		data, err := fn(json.RawMessage(body))
		if err != nil {
			writeErr(w, http.StatusBadGateway, err.Error())
			return
		}
		writeRawJSON(w, http.StatusOK, data)
	}
}

func (s *Server) omPathBodyProxy(fn func(*http.Request, json.RawMessage) (json.RawMessage, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		data, err := fn(r, json.RawMessage(body))
		if err != nil {
			writeErr(w, http.StatusBadGateway, err.Error())
			return
		}
		writeRawJSON(w, http.StatusOK, data)
	}
}

func (s *Server) omDeleteProxy(fn func(*http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := fn(r); err != nil {
			writeErr(w, http.StatusBadGateway, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) omIngestProxy() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := s.om.IngestEvent(json.RawMessage(body)); err != nil {
			writeErr(w, http.StatusBadGateway, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
