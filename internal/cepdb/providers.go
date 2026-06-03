package cepdb

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var httpClient = &http.Client{Timeout: 8 * time.Second}

func fetchViaCEP(ctx context.Context, cep string) (*CEP, error) {
	url := fmt.Sprintf("https://viacep.com.br/ws/%s/json/", cep)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var v struct {
		CEP         string `json:"cep"`
		Logradouro  string `json:"logradouro"`
		Complemento string `json:"complemento"`
		Bairro      string `json:"bairro"`
		Localidade  string `json:"localidade"`
		UF          string `json:"uf"`
		IBGE        string `json:"ibge"`
		GIA         string `json:"gia"`
		DDD         string `json:"ddd"`
		SIAFI       string `json:"siafi"`
		Erro        bool   `json:"erro"`
	}
	if err := json.Unmarshal(body, &v); err != nil {
		return nil, err
	}
	if v.Erro || v.CEP == "" {
		return nil, fmt.Errorf("cep not found")
	}

	return &CEP{
		CEP:         normalizeCEP(v.CEP),
		Logradouro:  v.Logradouro,
		Complemento: v.Complemento,
		Bairro:      v.Bairro,
		Localidade:  v.Localidade,
		UF:          strings.ToUpper(v.UF),
		IBGE:        v.IBGE,
		GIA:         v.GIA,
		DDD:         v.DDD,
		SIAFI:       v.SIAFI,
		Sources:     []string{"viacep"},
	}, nil
}

func fetchBrasilAPI(ctx context.Context, cep string) (*CEP, error) {
	url := fmt.Sprintf("https://brasilapi.com.br/api/cep/v2/%s", cep)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("brasilapi: status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var v struct {
		CEP          string  `json:"cep"`
		State        string  `json:"state"`
		City         string  `json:"city"`
		Neighborhood string  `json:"neighborhood"`
		Street       string  `json:"street"`
		IBGE         string  `json:"ibge"`
		Latitude     float64 `json:"latitude"`
		Longitude    float64 `json:"longitude"`
	}
	if err := json.Unmarshal(body, &v); err != nil {
		return nil, err
	}
	if v.CEP == "" {
		return nil, fmt.Errorf("cep not found")
	}

	return &CEP{
		CEP:        normalizeCEP(v.CEP),
		Logradouro: v.Street,
		Bairro:     v.Neighborhood,
		Localidade: v.City,
		UF:         strings.ToUpper(v.State),
		IBGE:       v.IBGE,
		Latitude:   v.Latitude,
		Longitude:  v.Longitude,
		Sources:    []string{"brasilapi"},
	}, nil
}

func fetchOpenCEP(ctx context.Context, cep string) (*CEP, error) {
	url := fmt.Sprintf("https://opencep.com/v1/%s", cep)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("opencep: status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var v struct {
		CEP         string `json:"cep"`
		Logradouro  string `json:"logradouro"`
		Complemento string `json:"complemento"`
		Bairro      string `json:"bairro"`
		Localidade  string `json:"localidade"`
		UF          string `json:"uf"`
		IBGE        string `json:"ibge"`
	}
	if err := json.Unmarshal(body, &v); err != nil {
		return nil, err
	}
	if v.CEP == "" {
		return nil, fmt.Errorf("cep not found")
	}

	return &CEP{
		CEP:         normalizeCEP(v.CEP),
		Logradouro:  v.Logradouro,
		Complemento: v.Complemento,
		Bairro:      v.Bairro,
		Localidade:  v.Localidade,
		UF:          strings.ToUpper(v.UF),
		IBGE:        v.IBGE,
		Sources:     []string{"opencep"},
	}, nil
}
