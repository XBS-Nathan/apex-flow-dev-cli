package phpimage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func baseCfg(t *testing.T, version string, extensions []string) ImageConfig {
	t.Helper()
	return ImageConfig{
		PHPVersion: version,
		Extensions: extensions,
	}
}

func TestGenerateDockerfile_BaseExtensionsOnly(t *testing.T) {
	t.Parallel()
	got := generateDockerfile(baseCfg(t, "8.2", nil))

	if !strings.Contains(got, "FROM php:8.2-fpm-bookworm") {
		t.Error("missing base image")
	}
	if !strings.Contains(got, "pdo_mysql") {
		t.Error("missing base extension pdo_mysql")
	}
	if strings.Contains(got, "pecl install imagick") {
		t.Error("should not contain extra extensions")
	}
}

func TestGenerateDockerfile_WithExtraExtensions(t *testing.T) {
	t.Parallel()
	got := generateDockerfile(baseCfg(t, "8.3", []string{"imagick", "swoole"}))

	if !strings.Contains(got, "FROM php:8.3-fpm-bookworm") {
		t.Error("missing base image")
	}
	if !strings.Contains(got, "pecl install imagick swoole") {
		t.Errorf("missing extra extensions in:\n%s", got)
	}
}

func TestGenerateDockerfile_WithNativeExtensions(t *testing.T) {
	t.Parallel()
	got := generateDockerfile(baseCfg(t, "8.2", []string{"gd", "zip", "intl", "exif"}))

	for _, dep := range []string{"libpng-dev", "libzip-dev", "icu-dev"} {
		if !strings.Contains(got, dep) {
			t.Errorf("missing build dep %q in:\n%s", dep, got)
		}
	}
	if !strings.Contains(got, "docker-php-ext-configure gd") {
		t.Error("missing gd configure step")
	}
	if !strings.Contains(got, "docker-php-ext-install "+baseExtensions+" exif gd intl zip") {
		t.Errorf("native extensions not in ext-install:\n%s", got)
	}
}

func TestGenerateDockerfile_MixedExtensions(t *testing.T) {
	t.Parallel()
	got := generateDockerfile(baseCfg(t, "8.3", []string{"gd", "imagick"}))

	if !strings.Contains(got, "docker-php-ext-install "+baseExtensions+" gd") {
		t.Errorf("native gd not in ext-install:\n%s", got)
	}
	if !strings.Contains(got, "pecl install imagick") {
		t.Errorf("pecl imagick missing:\n%s", got)
	}
}

func TestGenerateDockerfile_IncludesComposer(t *testing.T) {
	t.Parallel()
	got := generateDockerfile(baseCfg(t, "8.2", nil))

	if !strings.Contains(got, "COPY --from=composer:latest /usr/bin/composer /usr/bin/composer") {
		t.Errorf("missing composer COPY line in:\n%s", got)
	}
}

func TestUnionExtensions(t *testing.T) {
	t.Parallel()
	projects := [][]string{
		{"imagick", "swoole"},
		{"swoole", "mongodb"},
		nil,
	}

	got := unionExtensions(projects...)
	want := []string{"imagick", "mongodb", "swoole"}

	if len(got) != len(want) {
		t.Fatalf("unionExtensions() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("unionExtensions()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestImageHash(t *testing.T) {
	t.Parallel()
	h1 := imageHash(ImageConfig{Extensions: []string{"imagick", "swoole"}})
	h2 := imageHash(ImageConfig{Extensions: []string{"swoole", "imagick"}})
	h3 := imageHash(ImageConfig{Extensions: []string{"imagick"}})

	if h1 != h2 {
		t.Error("hash should be order-independent")
	}
	if h1 == h3 {
		t.Error("different extensions should produce different hashes")
	}
	if len(h1) != 8 {
		t.Errorf("hash length = %d, want 8", len(h1))
	}
}

func TestImageTag_FPM(t *testing.T) {
	t.Parallel()
	tag := ImageTag(ImageConfig{PHPVersion: "8.3", Runtime: "fpm"})
	if !strings.HasPrefix(tag, "nova-fpm:8.3-") {
		t.Errorf("ImageTag = %q, want prefix nova-fpm:8.3-", tag)
	}
}

func TestImageTag_FrankenPHP(t *testing.T) {
	t.Parallel()
	tag := ImageTag(ImageConfig{PHPVersion: "8.3", Runtime: "frankenphp"})
	if !strings.HasPrefix(tag, "nova-frankenphp:8.3-") {
		t.Errorf("ImageTag = %q, want prefix nova-frankenphp:8.3-", tag)
	}
}

func TestImageTag_DefaultRuntimeIsFPM(t *testing.T) {
	t.Parallel()
	// Empty Runtime should be treated as fpm so existing callers keep working.
	tag := ImageTag(ImageConfig{PHPVersion: "8.3"})
	if !strings.HasPrefix(tag, "nova-fpm:8.3-") {
		t.Errorf("ImageTag = %q, want prefix nova-fpm:8.3-", tag)
	}
}

func TestImageTag_RuntimesProduceDifferentTags(t *testing.T) {
	t.Parallel()
	fpm := ImageTag(ImageConfig{PHPVersion: "8.3", Runtime: "fpm", Extensions: []string{"gd"}})
	franken := ImageTag(ImageConfig{PHPVersion: "8.3", Runtime: "frankenphp", Extensions: []string{"gd"}})
	if fpm == franken {
		t.Errorf("expected different tags, got %q for both", fpm)
	}
}

func TestGenerateDockerfile_FPM_BaseImage(t *testing.T) {
	t.Parallel()
	df := generateDockerfile(ImageConfig{PHPVersion: "8.3", Runtime: "fpm"})
	if !strings.Contains(df, "FROM php:8.3-fpm-bookworm") {
		t.Errorf("FPM Dockerfile missing fpm base, got:\n%s", df)
	}
	if !strings.Contains(df, "php-fpm.d/www.conf") {
		t.Errorf("FPM Dockerfile missing www.conf strip, got:\n%s", df)
	}
}

func TestGenerateDockerfile_FrankenPHP_BaseImage(t *testing.T) {
	t.Parallel()
	df := generateDockerfile(ImageConfig{PHPVersion: "8.3", Runtime: "frankenphp"})
	if !strings.Contains(df, "FROM dunglas/frankenphp:1-php8.3-bookworm") {
		t.Errorf("FrankenPHP Dockerfile missing frankenphp base, got:\n%s", df)
	}
	if strings.Contains(df, "php-fpm.d/www.conf") {
		t.Errorf("FrankenPHP Dockerfile should not strip www.conf, got:\n%s", df)
	}
	if !strings.Contains(df, "COPY Caddyfile /etc/caddy/Caddyfile") {
		t.Errorf("FrankenPHP Dockerfile missing Caddyfile copy, got:\n%s", df)
	}
}

func TestGenerateDockerfile_FrankenPHP_KeepsExtensionInstall(t *testing.T) {
	t.Parallel()
	df := generateDockerfile(ImageConfig{PHPVersion: "8.3", Runtime: "frankenphp", Extensions: []string{"gd"}})
	if !strings.Contains(df, "docker-php-ext-install") {
		t.Errorf("FrankenPHP Dockerfile missing docker-php-ext-install, got:\n%s", df)
	}
	if !strings.Contains(df, "pecl install") {
		t.Errorf("FrankenPHP Dockerfile missing pecl install, got:\n%s", df)
	}
}

func TestWriteDockerfile_FrankenPHPWritesCaddyfile(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // GlobalDir() resolves under HOME; cannot use t.Parallel with t.Setenv

	dir, err := writeDockerfile(ImageConfig{PHPVersion: "8.3", Runtime: "frankenphp"})
	if err != nil {
		t.Fatalf("writeDockerfile: %v", err)
	}

	caddyfilePath := filepath.Join(dir, "Caddyfile")
	data, err := os.ReadFile(caddyfilePath)
	if err != nil {
		t.Fatalf("reading Caddyfile: %v", err)
	}
	content := string(data)
	for _, want := range []string{"frankenphp", "auto_https off", "admin off", "{$NOVA_APP}", "php_server"} {
		if !strings.Contains(content, want) {
			t.Errorf("Caddyfile missing %q, got:\n%s", want, content)
		}
	}
}

func TestWriteDockerfile_FPMDoesNotWriteCaddyfile(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // cannot use t.Parallel with t.Setenv

	dir, err := writeDockerfile(ImageConfig{PHPVersion: "8.3", Runtime: "fpm"})
	if err != nil {
		t.Fatalf("writeDockerfile: %v", err)
	}

	switch _, err := os.Stat(filepath.Join(dir, "Caddyfile")); {
	case err == nil:
		t.Error("FPM should not write Caddyfile, but it exists")
	case !os.IsNotExist(err):
		t.Errorf("FPM Caddyfile stat: unexpected error: %v", err)
	}
}
