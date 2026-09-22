package sandbox

import "sync"

// tokenStore remembers the session token installed in each running sandbox.
// A second attach on one name must return the token the sandbox already
// accepts, because the run hook installs it once and a running sandbox cannot
// be given another.
//
// A token is generated on demand for a name the store does not hold, which
// covers a sandbox this process did not start.
type tokenStore struct {
	mu     sync.Mutex
	tokens map[string]string
}

func newTokenStore() *tokenStore {
	return &tokenStore{tokens: map[string]string{}}
}

func (s *tokenStore) get(name string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if token, ok := s.tokens[name]; ok {
		return token
	}
	token, err := NewSecret()
	if err != nil {
		return ""
	}
	s.tokens[name] = token
	return token
}

func (s *tokenStore) set(name, token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens[name] = token
}

func (s *tokenStore) forget(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tokens, name)
}
