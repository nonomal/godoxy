package agent

import (
	"bytes"
	"text/template"
)

var (
	installScript = `AGENT_NAME="{{.Name}}" \
	AGENT_PORT="{{.Port}}" \
	AGENT_CA_CERT="{{.CACert}}" \
	AGENT_SSL_CERT="{{.SSLCert}}" \
	bash -c "$(curl -fsSL https://raw.githubusercontent.com/yusing/godoxy/main/scripts/install-agent.sh)"`
	installScriptTemplate = template.Must(template.New("install.sh").Parse(installScript))
)

func (c *AgentEnvConfig) Generate() (string, error) {
	buf := bytes.NewBuffer(make([]byte, 0, 4096))
	if err := installScriptTemplate.Execute(buf, c); err != nil {
		return "", err
	}
	return buf.String(), nil
}
