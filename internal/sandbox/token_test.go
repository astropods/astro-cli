package sandbox

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testSecret  = "0123456789abcdef0123456789abcdef"
	testBroker  = "http://host.docker.internal:3199"
	testDeploy  = "local"
	otherSecret = "ffffffffffffffffffffffffffffffff"
)

func TestSignedTokenCarriesTheBrokerAsIssuerSoTheSDKFindsIt(t *testing.T) {
	signed, err := SignToken(testDeploy, testBroker, testSecret)
	require.NoError(t, err)

	parsed, _, err := jwt.NewParser().ParseUnverified(signed, &jwt.RegisteredClaims{})
	require.NoError(t, err)
	claims, ok := parsed.Claims.(*jwt.RegisteredClaims)
	require.True(t, ok)

	assert.Equal(t, testBroker, claims.Issuer,
		"the SDK takes the control-plane base URL from iss, so the agent needs no second variable")
	assert.Equal(t, testDeploy, claims.Subject)
	assert.Equal(t, "HS256", parsed.Method.Alg())
}

func TestVerifyTokenRoundTripsAndReturnsTheDeployment(t *testing.T) {
	signed, err := SignToken(testDeploy, testBroker, testSecret)
	require.NoError(t, err)

	deployment, err := VerifyToken(signed, testSecret)
	require.NoError(t, err)
	assert.Equal(t, testDeploy, deployment)
}

func TestVerifyTokenRefusesTokensItShould(t *testing.T) {
	valid, err := SignToken(testDeploy, testBroker, testSecret)
	require.NoError(t, err)

	noSubject, err := jwt.NewWithClaims(jwt.SigningMethodHS256,
		jwt.RegisteredClaims{Issuer: testBroker}).SignedString([]byte(testSecret))
	require.NoError(t, err)

	unsigned, err := jwt.NewWithClaims(jwt.SigningMethodNone,
		jwt.RegisteredClaims{Subject: testDeploy}).SignedString(jwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err)

	for _, tc := range []struct{ name, token, secret string }{
		{"a token from another run", valid, otherSecret},
		{"a tampered payload", valid[:len(valid)-4] + "AAAA", testSecret},
		{"a token with no subject", noSubject, testSecret},
		{"an unsigned token", unsigned, testSecret},
		{"not a token at all", "not-a-jwt", testSecret},
		{"an empty string", "", testSecret},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := VerifyToken(tc.token, tc.secret)
			assert.ErrorIs(t, err, ErrInvalidToken)
		})
	}
}

func TestDeploymentFromRequestReadsABearerTokenTheWayProductionDoes(t *testing.T) {
	signed, err := SignToken(testDeploy, testBroker, testSecret)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/sandboxes/conv-1", nil)
	req.Header.Set("Authorization", "Bearer "+signed)

	deployment, err := DeploymentFromRequest(req, testSecret)
	require.NoError(t, err)
	assert.Equal(t, testDeploy, deployment)
}

func TestDeploymentFromRequestSeparatesMissingFromInvalid(t *testing.T) {
	signed, err := SignToken(testDeploy, testBroker, testSecret)
	require.NoError(t, err)

	for _, tc := range []struct {
		name   string
		header string
		want   error
	}{
		{"no header", "", ErrMissingToken},
		{"an empty bearer", "Bearer ", ErrMissingToken},
		{"another scheme", "Basic abcdef", ErrMissingToken},
		{"a bare token with no scheme", signed, ErrMissingToken},
		{"a token from another run", "Bearer " + mustSign(t, otherSecret), ErrInvalidToken},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPut, "/api/v1/sandboxes/conv-1", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			_, err := DeploymentFromRequest(req, testSecret)
			assert.ErrorIs(t, err, tc.want,
				"the local broker must separate these the way production does, or an author debugs the wrong fault")
		})
	}
}

func mustSign(t *testing.T, secret string) string {
	t.Helper()
	signed, err := SignToken(testDeploy, testBroker, secret)
	require.NoError(t, err)
	return signed
}

func TestWriteAuthErrorMatchesTheDeployedBody(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"missing", ErrMissingToken, "missing deploy token"},
		{"invalid", ErrInvalidToken, "invalid deploy token"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			WriteAuthError(rec, tc.err)

			assert.Equal(t, http.StatusUnauthorized, rec.Code)
			assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

			var body map[string]string
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			assert.Equal(t, tc.want, body["error"],
				"astro-server writes this exact string, and a client that reads it must see one value")
		})
	}
}

func TestNewSecretIsLongAndDiffersEachRun(t *testing.T) {
	first, err := NewSecret()
	require.NoError(t, err)
	second, err := NewSecret()
	require.NoError(t, err)

	assert.Len(t, first, 64)
	assert.NotEqual(t, first, second,
		"a per-run secret stops a token from one `ast dev` being replayed against the next")
	assert.False(t, strings.Contains(first, " "))
}
