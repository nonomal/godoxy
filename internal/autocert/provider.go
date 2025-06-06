package autocert

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"maps"
	"os"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/registration"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/yusing/go-proxy/internal/common"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/notif"
	"github.com/yusing/go-proxy/internal/task"
	"github.com/yusing/go-proxy/internal/utils/strutils"
)

type (
	Provider struct {
		cfg     *Config
		user    *User
		legoCfg *lego.Config
		client  *lego.Client

		legoCert     *certificate.Resource
		tlsCert      *tls.Certificate
		certExpiries CertExpiries
	}

	CertExpiries map[string]time.Time
)

var ErrGetCertFailure = errors.New("get certificate failed")

func NewProvider(cfg *Config, user *User, legoCfg *lego.Config) *Provider {
	return &Provider{
		cfg:     cfg,
		user:    user,
		legoCfg: legoCfg,
	}
}

func (p *Provider) GetCert(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
	if p.tlsCert == nil {
		return nil, ErrGetCertFailure
	}
	return p.tlsCert, nil
}

func (p *Provider) GetName() string {
	return p.cfg.Provider
}

func (p *Provider) GetCertPath() string {
	return p.cfg.CertPath
}

func (p *Provider) GetKeyPath() string {
	return p.cfg.KeyPath
}

func (p *Provider) GetExpiries() CertExpiries {
	return p.certExpiries
}

func (p *Provider) ObtainCert() error {
	if p.cfg.Provider == ProviderLocal {
		return nil
	}

	if p.cfg.Provider == ProviderPseudo {
		log.Info().Msg("init client for pseudo provider")
		<-time.After(time.Second)
		log.Info().Msg("registering acme for pseudo provider")
		<-time.After(time.Second)
		log.Info().Msg("obtained cert for pseudo provider")
		return nil
	}

	if p.client == nil {
		if err := p.initClient(); err != nil {
			return err
		}
	}

	if p.user.Registration == nil {
		if err := p.registerACME(); err != nil {
			return err
		}
	}

	var cert *certificate.Resource
	var err error

	if p.legoCert != nil {
		cert, err = p.client.Certificate.RenewWithOptions(*p.legoCert, &certificate.RenewOptions{
			Bundle: true,
		})
		if err != nil {
			p.legoCert = nil
			log.Err(err).Msg("cert renew failed, fallback to obtain")
		} else {
			p.legoCert = cert
		}
	}

	if cert == nil {
		cert, err = p.client.Certificate.Obtain(certificate.ObtainRequest{
			Domains: p.cfg.Domains,
			Bundle:  true,
		})
		if err != nil {
			return err
		}
	}

	if err = p.saveCert(cert); err != nil {
		return err
	}

	tlsCert, err := tls.X509KeyPair(cert.Certificate, cert.PrivateKey)
	if err != nil {
		return err
	}

	expiries, err := getCertExpiries(&tlsCert)
	if err != nil {
		return err
	}
	p.tlsCert = &tlsCert
	p.certExpiries = expiries

	return nil
}

func (p *Provider) LoadCert() error {
	cert, err := tls.LoadX509KeyPair(p.cfg.CertPath, p.cfg.KeyPath)
	if err != nil {
		return fmt.Errorf("load SSL certificate: %w", err)
	}
	expiries, err := getCertExpiries(&cert)
	if err != nil {
		return fmt.Errorf("parse SSL certificate: %w", err)
	}
	p.tlsCert = &cert
	p.certExpiries = expiries

	log.Info().Msgf("next renewal in %v", strutils.FormatDuration(time.Until(p.ShouldRenewOn())))
	return p.renewIfNeeded()
}

// ShouldRenewOn returns the time at which the certificate should be renewed.
func (p *Provider) ShouldRenewOn() time.Time {
	for _, expiry := range p.certExpiries {
		return expiry.AddDate(0, -1, 0) // 1 month before
	}
	// this line should never be reached
	panic("no certificate available")
}

func (p *Provider) ScheduleRenewal(parent task.Parent) {
	if p.GetName() == ProviderLocal || p.GetName() == ProviderPseudo {
		return
	}
	go func() {
		lastErrOn := time.Time{}
		renewalTime := p.ShouldRenewOn()
		timer := time.NewTimer(time.Until(renewalTime))
		defer timer.Stop()

		task := parent.Subtask("cert-renew-scheduler", true)
		defer task.Finish(nil)

		for {
			select {
			case <-task.Context().Done():
				return
			case <-timer.C:
				// Retry after 1 hour on failure
				if !lastErrOn.IsZero() && time.Now().Before(lastErrOn.Add(time.Hour)) {
					continue
				}
				if err := p.renewIfNeeded(); err != nil {
					gperr.LogWarn("cert renew failed", err)
					lastErrOn = time.Now()
					notif.Notify(&notif.LogMessage{
						Level: zerolog.ErrorLevel,
						Title: "SSL certificate renewal failed",
						Body:  notif.MessageBody(err.Error()),
					})
					continue
				}
				notif.Notify(&notif.LogMessage{
					Level: zerolog.InfoLevel,
					Title: "SSL certificate renewed",
					Body:  notif.ListBody(p.cfg.Domains),
				})
				// Reset on success
				lastErrOn = time.Time{}
				renewalTime = p.ShouldRenewOn()
				timer.Reset(time.Until(renewalTime))
			}
		}
	}()
}

func (p *Provider) initClient() error {
	legoClient, err := lego.NewClient(p.legoCfg)
	if err != nil {
		return err
	}

	err = legoClient.Challenge.SetDNS01Provider(p.cfg.challengeProvider)
	if err != nil {
		return err
	}

	p.client = legoClient
	return nil
}

func (p *Provider) registerACME() error {
	if p.user.Registration != nil {
		return nil
	}
	if reg, err := p.client.Registration.ResolveAccountByKey(); err == nil {
		p.user.Registration = reg
		log.Info().Msg("reused acme registration from private key")
		return nil
	}

	reg, err := p.client.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
	if err != nil {
		return err
	}
	p.user.Registration = reg
	log.Info().Interface("reg", reg).Msg("acme registered")
	return nil
}

func (p *Provider) saveCert(cert *certificate.Resource) error {
	if common.IsTest {
		return nil
	}
	/* This should have been done in setup
	but double check is always a good choice.*/
	_, err := os.Stat(path.Dir(p.cfg.CertPath))
	if err != nil {
		if os.IsNotExist(err) {
			if err = os.MkdirAll(path.Dir(p.cfg.CertPath), 0o755); err != nil {
				return err
			}
		} else {
			return err
		}
	}
	err = os.WriteFile(p.cfg.KeyPath, cert.PrivateKey, 0o600) // -rw-------
	if err != nil {
		return err
	}

	err = os.WriteFile(p.cfg.CertPath, cert.Certificate, 0o644) // -rw-r--r--
	if err != nil {
		return err
	}
	return nil
}

func (p *Provider) certState() CertState {
	if time.Now().After(p.ShouldRenewOn()) {
		return CertStateExpired
	}

	if len(p.certExpiries) != len(p.cfg.Domains) {
		return CertStateMismatch
	}

	for i := range len(p.cfg.Domains) {
		if _, ok := p.certExpiries[p.cfg.Domains[i]]; !ok {
			log.Info().Msgf("autocert domains mismatch: cert: %s, wanted: %s",
				strings.Join(slices.Collect(maps.Keys(p.certExpiries)), ", "),
				strings.Join(p.cfg.Domains, ", "))
			return CertStateMismatch
		}
	}

	return CertStateValid
}

func (p *Provider) renewIfNeeded() error {
	if p.cfg.Provider == ProviderLocal {
		return nil
	}

	switch p.certState() {
	case CertStateExpired:
		log.Info().Msg("certs expired, renewing")
	case CertStateMismatch:
		log.Info().Msg("cert domains mismatch with config, renewing")
	default:
		return nil
	}

	return p.ObtainCert()
}

func getCertExpiries(cert *tls.Certificate) (CertExpiries, error) {
	r := make(CertExpiries, len(cert.Certificate))
	for _, cert := range cert.Certificate {
		x509Cert, err := x509.ParseCertificate(cert)
		if err != nil {
			return nil, err
		}
		if x509Cert.IsCA {
			continue
		}
		r[x509Cert.Subject.CommonName] = x509Cert.NotAfter
		for i := range x509Cert.DNSNames {
			r[x509Cert.DNSNames[i]] = x509Cert.NotAfter
		}
	}
	return r, nil
}
