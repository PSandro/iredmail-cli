package iredmail

import (
	"database/sql"
	"fmt"
	"strings"
)

const (
	// DomainDefaultSettings keep the defaut settings
	DomainDefaultSettings = "default_user_quota:2048"
	domainQueryAll        = ""
	domainQueryByDomain   = "WHERE domain = ?"
)

// Domain struct
type Domain struct {
	Domain      string
	Description string
	Settings    string
	Aliases     DomainAliases
	Catchalls   Forwardings
}

// Domains ...
type Domains []Domain

// FilterBy is method that filters Domains by a given string
func (d Domains) FilterBy(filter string) Domains {
	filteredDomains := Domains{}

	for _, domain := range d {
		if strings.Contains(domain.Domain, filter) ||
			strings.Contains(domain.Description, filter) ||
			len(domain.Aliases.FilterBy(filter)) > 0 ||
			len(domain.Catchalls.FilterBy(filter)) > 0 {
			filteredDomains = append(filteredDomains, domain)
		}
	}

	return filteredDomains
}

func (s *Server) domainQuery(whereQuery string, args ...interface{}) (Domains, error) {
	domains := Domains{}

	sqlQuery := `
	SELECT domain, description, settings FROM domain 
	` + whereQuery + `
	ORDER BY domain ASC;`

	rows, err := s.DB.Query(sqlQuery, args...)
	if err != nil {
		return domains, err
	}
	defer rows.Close()

	for rows.Next() {
		var domain string
		var descriptionString, settingsString sql.NullString

		err := rows.Scan(&domain, &descriptionString, &settingsString)
		if err != nil {
			return domains, err
		}

		domainAliases, err := s.domainAliasQuery(domainAliasQueryByDomain, domain)
		if err != nil {
			return domains, err
		}

		catchalls, err := s.forwardingQuery(forwardingQueryCatchallByDomain, domain)
		if err != nil {
			return domains, err
		}
		var description, settings string

		if descriptionString.Valid {
			description = descriptionString.String
		}

		if settingsString.Valid {
			settings = settingsString.String
		}

		domains = append(domains, Domain{
			Domain:      domain,
			Description: description,
			Settings:    settings,
			Aliases:     domainAliases,
			Catchalls:   catchalls,
		})

	}
	err = rows.Err()

	return domains, err
}

func (s *Server) domainExists(domain string) (bool, error) {
	var exists bool

	sqlQuery := `
	SELECT exists
	(SELECT * FROM domain
	WHERE domain = ?);`

	err := s.DB.QueryRow(sqlQuery, domain).Scan(&exists)

	return exists, err
}

// Domains returns all Domains
func (s *Server) Domains() (Domains, error) {
	return s.domainQuery(domainQueryAll)
}

// Domain returns a Domain by its domain name
func (s *Server) Domain(domainName string) (Domain, error) {
	domain := Domain{}

	domainExists, err := s.domainExists(domainName)
	if err != nil {
		return domain, err
	}
	if !domainExists {
		return domain, fmt.Errorf("Domain %s doesn't exist", domainName)
	}

	domaines, err := s.domainQuery(domainQueryByDomain, domainName)
	if err != nil {
		return domain, err
	}

	if len(domaines) == 0 {
		return domain, fmt.Errorf("Domain not found")
	}

	return domaines[0], nil
}

// DomainAdd adds a new domain
func (s *Server) DomainAdd(domain Domain) error {
	exists, err := s.domainExists(domain.Domain)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("Domain %s already exists", domain.Domain)
	}

	if domain.Settings == "" {
		domain.Settings = DomainDefaultSettings
	}

	sqlQuery := `
	INSERT INTO domain (domain, description, settings, active)
	VALUES (?, ?, ?, 1);`
	_, err = s.DB.Exec(sqlQuery, domain.Domain, domain.Description, domain.Settings)

	return err
}

// DomainDelete deletes a domain
func (s *Server) DomainDelete(domain string) error {
	exists, err := s.domainExists(domain)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("Domain %s doesn't exist", domain)
	}

	domainMailboxes, err := s.mailboxQuery(mailboxQueryByDomain, domain)
	if err != nil {
		return err
	}
	if len(domainMailboxes) > 0 {
		return fmt.Errorf("There are still mailboxes with the domain %s, you need to delete them before", domain)
	}

	aliases, err := s.aliasQuery(aliasQueryByDomain, domain)
	if err != nil {
		return err
	}
	if len(aliases) > 0 {
		return fmt.Errorf("There are still aliases from the domain %s, you need to delete them before", domain)
	}

	domainAliases, err := s.domainAliasQuery(domainAliasQueryByDomain, domain)
	if err != nil {
		return err
	}
	if len(domainAliases) > 0 {
		err = s.domainAliasDeleteAll(domain)
		if err != nil {
			return err
		}
	}

	domainCatchalls, err := s.forwardingQuery(forwardingQueryCatchallByDomain, domain)
	if err != nil {
		return err
	}
	if len(domainCatchalls) > 0 {
		err = s.domainCatchallDeleteAll(domain)
		if err != nil {
			return err
		}
	}

	sqlQuery := "DELETE FROM domain WHERE domain = ?;"
	_, err = s.DB.Exec(sqlQuery, domain)

	return err
}
