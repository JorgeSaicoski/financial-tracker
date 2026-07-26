package dto

// ConfigResponse is what GET /config returns: the minimum the frontend
// needs to decide whether to show the login guard before it renders
// anything else. See FRONT-04's ticket and the handler's doc comment for
// why these two fields are the seed of a larger picture BACK-09
// (standalone) and BACK-02 (auth) will eventually own outright.
type ConfigResponse struct {
	Standalone  bool `json:"standalone"`
	AuthEnabled bool `json:"auth_enabled"`
}
