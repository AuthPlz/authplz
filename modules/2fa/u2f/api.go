package u2f

import (
    "fmt"
    "log"
    "time"
    "net/http"
    "encoding/json"
)
import "github.com/gocraft/web"
import "github.com/ryankurte/go-u2f"

import "github.com/ryankurte/authplz/controllers/datastore"
import "github.com/ryankurte/authplz/api"

func init() {
    gob.Register(&Challenge{})
}

func (c *U2FCtx) U2FEnrolPost(rw web.ResponseWriter, req *web.Request) {

    // Check if user is logged in
    if c.userid == "" {
        c.WriteApiResult(rw, api.ApiResultError, api.GetApiLocale(c.locale).Unauthorized)
        return
    }

    // Fetch request from session vars
    // TODO: move this to a separate session flash
    if c.session.Values["u2f-register-challenge"] == nil {
        c.WriteApiResult(rw, api.ApiResultError, "No challenge found")
        fmt.Println("No challenge found in session flash")
        return
    }
    challenge := c.session.Values["u2f-register-challenge"].(*u2f.Challenge)
    c.session.Values["u2f-register-challenge"] = ""

    // Parse JSON response body
    var u2fResp u2f.RegisterResponse
    jsonErr := json.NewDecoder(req.Body).Decode(&u2fResp)
    if jsonErr != nil {
        c.WriteApiResult(rw, api.ApiResultError, "Invalid U2F registration response")
        return
    }

    // Check registration validity
    // TODO: attestation should be disabled only in test mode, need a better certificate list
    reg, err := challenge.Register(u2fResp, &u2f.RegistrationConfig{SkipAttestationVerify: true})
    if err != nil {
        // Registration failed.
        log.Println(err)
        c.WriteApiResult(rw, api.ApiResultError, api.GetApiLocale(c.locale).U2FRegistrationFailed)
        return
    }

    // Create datastore token model
    token := datastore.FidoToken{
        KeyHandle:   reg.KeyHandle,
        PublicKey:   reg.PublicKey,
        Certificate: reg.Certificate,
        Counter:     reg.Counter,
    }

    // Save registration against user
    _, err = c.global.userController.AddFidoToken(c.userid, &token)
    if err != nil {
        // Registration failed.
        log.Println(err)
        c.WriteApiResult(rw, api.ApiResultError, api.GetApiLocale(c.locale).InternalError)
        return
    }

    log.Printf("Enrolled U2F token for account %s\n", c.userid)
    c.WriteApiResult(rw, api.ApiResultOk, api.GetApiLocale(c.locale).U2FRegistrationComplete)
}

func (c *U2FCtx) U2FBindAuthenticationRequest(rw web.ResponseWriter, req *web.Request, userid string) {
    u2fSession, err := c.global.sessionStore.Get(req.Request, "u2f-sign-session")
    if err != nil {
        log.Printf("Error fetching u2f-sign-session 1 %s", err)
        c.WriteApiResult(rw, api.ApiResultError, api.GetApiLocale(c.locale).InternalError)
        return
    }

    log.Printf("U2F adding authorization flash for user %s\n", userid)

    u2fSession.Values["u2f-sign-userid"] = userid
    u2fSession.Save(req.Request, rw)
}

func (c *U2FCtx) U2FAuthenticateGet(rw web.ResponseWriter, req *web.Request) {
    u2fSession, err := c.global.sessionStore.Get(req.Request, "u2f-sign-session")
    if err != nil {
        log.Printf("Error fetching u2f-sign-session 2 %s", err)
        c.WriteApiResult(rw, api.ApiResultError, api.GetApiLocale(c.locale).InternalError)
        return
    }

    if u2fSession.Values["u2f-sign-userid"] == nil {
        c.WriteApiResult(rw, api.ApiResultError, "No userid found")
        fmt.Println("No userid found in session flash")
        return
    }
    userid := u2fSession.Values["u2f-sign-userid"].(string)

    log.Printf("U2F Authenticate request for user %s", userid)

    // Fetch existing keys
    tokens, err := c.global.userController.GetFidoTokens(userid)
    if err != nil {
        log.Printf("Error fetching U2F tokens %s", err)
        c.WriteApiResult(rw, api.ApiResultError, api.GetApiLocale(c.locale).InternalError)
        return
    }

    //Coerce to U2F types
    var registeredKeys []u2f.Registration
    for _, v := range tokens {
        reg := u2f.Registration{
            KeyHandle:   v.KeyHandle,
            PublicKey:   v.PublicKey,
            Certificate: v.Certificate,
            Counter:     v.Counter,
        }
        registeredKeys = append(registeredKeys, reg)
    }

    // Build U2F challenge
    challenge, err := u2f.NewChallenge(c.global.url, []string{c.global.url}, registeredKeys)
    if err != nil {
        log.Printf("Error creating U2F sign request %s", err)
        c.WriteApiResult(rw, api.ApiResultError, api.GetApiLocale(c.locale).InternalError)
        return
    }

    u2fSignReq := challenge.SignRequest()

    u2fSession.Values["u2f-sign-challenge"] = challenge
    u2fSession.Save(req.Request, rw)

    c.WriteJson(rw, *u2fSignReq)
}

func (c *U2FCtx) U2FAuthenticatePost(rw web.ResponseWriter, req *web.Request) {

    u2fSession, err := c.global.sessionStore.Get(req.Request, "u2f-sign-session")
    if err != nil {
        log.Printf("Error fetching u2f-sign-session 3  %s", err)
        c.WriteApiResult(rw, api.ApiResultError, api.GetApiLocale(c.locale).InternalError)
        return
    }

    // Fetch request from session vars
    // TODO: move this to a separate session flash
    if u2fSession.Values["u2f-sign-challenge"] == nil {
        c.WriteApiResult(rw, api.ApiResultError, "No challenge found")
        fmt.Println("No challenge found in session flash")
        return
    }
    challenge := u2fSession.Values["u2f-sign-challenge"].(*u2f.Challenge)
    u2fSession.Values["u2f-sign-challenge"] = ""

    if u2fSession.Values["u2f-sign-userid"] == nil {
        c.WriteApiResult(rw, api.ApiResultError, "No userid found")
        fmt.Println("No userid found in session flash")
        return
    }
    userid := u2fSession.Values["u2f-sign-userid"].(string)
    u2fSession.Values["u2f-sign-userid"] = ""

    u2fSession.Save(req.Request, rw)

    // Parse JSON response body
    var u2fSignResp u2f.SignResponse
    jsonErr := json.NewDecoder(req.Body).Decode(&u2fSignResp)
    if jsonErr != nil {
        c.WriteApiResult(rw, api.ApiResultError, "Invalid U2F registration response")
        return
    }

    // Fetch user object
    u, err := c.global.userController.GetUser(userid)
    if err != nil {
        log.Println(err)
        c.WriteApiResult(rw, api.ApiResultError, api.GetApiLocale(c.locale).InternalError)
        return
    }

    // Check signature validity
    reg, err := challenge.Authenticate(u2fSignResp)
    if err != nil {
        // Registration failed.
        log.Println(err)
        c.WriteApiResult(rw, api.ApiResultError, api.GetApiLocale(c.locale).U2FRegistrationFailed)
        return
    }

    // Fetch existing keys
    tokens, err := c.global.userController.GetFidoTokens(userid)
    if err != nil {
        log.Printf("Error fetching U2F tokens %s", err)
        c.WriteApiResult(rw, api.ApiResultError, api.GetApiLocale(c.locale).InternalError)
        return
    }

    // Match with registration token
    var token *datastore.FidoToken = nil
    for _, t := range tokens {
        if t.KeyHandle == reg.KeyHandle {
            token = &t
        }
    }
    if token == nil {
        log.Printf("Matching U2F token not found")
        c.WriteApiResult(rw, api.ApiResultError, api.GetApiLocale(c.locale).NoU2FTokenFound)
        return
    }

    // Update token counter / last used
    token.Counter = reg.Counter
    token.LastUsed = time.Now()

    // Save updated token against user
    err = c.global.userController.UpdateFidoToken(token)
    if err != nil {
        // Registration failed.
        log.Println(err)
        c.WriteApiResult(rw, api.ApiResultError, api.GetApiLocale(c.locale).InternalError)
        return
    }

    log.Printf("Valid U2F login for account %s\n", userid)
    c.LoginUser(u, rw, req)
    c.WriteApiResult(rw, api.ApiResultOk, api.GetApiLocale(c.locale).LoginSuccessful)
}

func (c *U2FCtx) U2FTokensGet(rw web.ResponseWriter, req *web.Request) {

    // Check if user is logged in
    if c.userid == "" {
        c.WriteApiResult(rw, api.ApiResultError, api.GetApiLocale(c.locale).Unauthorized)
        return
    }

    tokens, err := c.global.userController.GetFidoTokens(c.userid)
    if err != nil {
        log.Printf("Error fetching U2F tokens %s", err)
        c.WriteApiResult(rw, api.ApiResultError, api.GetApiLocale(c.locale).InternalError)
        return
    }

    c.WriteJson(rw, tokens)
}