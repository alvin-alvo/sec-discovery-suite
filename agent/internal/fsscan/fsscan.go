package fsscan

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"sds/agent/internal/attribution"
	"sds/agent/internal/certinfo"
	"software.sslmate.com/src/go-pkcs12"
)

// DefaultRoots defines standard directories to scan per docs/02.
var DefaultRoots = []string{
	"/etc/ssl",
	"/usr/share/ca-certificates",
	"/etc/pki",
	"/etc/letsencrypt",
	"/etc/nginx",
	"/etc/apache2",
	"/opt",
}

const maxFileSize = 1024 * 1024 // 1 MB limit

// CertObservation represents the docs/06 cert_observations wire format record.
type CertObservation struct {
	Path               string                `json:"path"`
	ResolvedPath       *string               `json:"resolved_path"`
	Filename           string                `json:"filename"`
	Format             string                `json:"format"`
	ScanRoot           string                `json:"scan_root"`
	ChainPosition      int                   `json:"chain_position"`
	ChainLength        int                   `json:"chain_length"`
	FileMode           string                `json:"file_mode"`
	FileSize           int64                 `json:"file_size"`
	IsSymlink          bool                  `json:"is_symlink"`
	ContainsPrivateKey bool                  `json:"contains_private_key"`
	ParseStatus        string                `json:"parse_status"`
	ParseError         *string               `json:"parse_error"`
	DaysToExpiry       *int                  `json:"days_to_expiry"`
	ValidityStatus     *string               `json:"validity_status"`
	Owner              *attribution.Owner    `json:"owner"`
	DiscoveredAt       string                `json:"discovered_at"`
	Certificate        *certinfo.Certificate `json:"certificate"`
}

// ScanError represents an error entry per docs/06.
type ScanError struct {
	Code    string  `json:"code"`
	Message string  `json:"message"`
	Path    *string `json:"path"`
}

// ScanResult holds all observations and errors discovered during the filesystem scan.
type ScanResult struct {
	Observations []*CertObservation `json:"observations"`
	Errors       []*ScanError       `json:"errors"`
}

type fileJob struct {
	path     string
	scanRoot string
}

// Scan walks the specified roots and extracts certificate observations.
func Scan(roots []string, now time.Time) (*ScanResult, error) {
	if len(roots) == 0 {
		roots = DefaultRoots
	}

	var jobs []fileJob
	var rootErrors []*ScanError
	var rootObservations []*CertObservation

	skipDirs := map[string]bool{
		"/proc": true,
		"/sys":  true,
		"/dev":  true,
		"/run":  true,
	}

	for _, root := range roots {
		cleanRoot := filepath.Clean(root)
		if skipDirs[cleanRoot] {
			continue
		}

		// Check if root exists
		_, err := os.Lstat(cleanRoot)
		if err != nil {
			if os.IsPermission(err) {
				p := cleanRoot
				rootErrors = append(rootErrors, &ScanError{
					Code:    "access_denied",
					Message: err.Error(),
					Path:    &p,
				})
				rootObservations = append(rootObservations, &CertObservation{
					Path:          cleanRoot,
					Filename:      filepath.Base(cleanRoot),
					Format:        "UNKNOWN",
					ScanRoot:      cleanRoot,
					ChainPosition: -1,
					ParseStatus:   "access_denied",
					DiscoveredAt:  now.UTC().Format(time.RFC3339),
				})
			}
			continue
		}

		err = filepath.Walk(cleanRoot, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				if os.IsPermission(err) {
					p := path
					rootErrors = append(rootErrors, &ScanError{
						Code:    "access_denied",
						Message: err.Error(),
						Path:    &p,
					})
					rootObservations = append(rootObservations, &CertObservation{
						Path:          path,
						Filename:      filepath.Base(path),
						Format:        "UNKNOWN",
						ScanRoot:      cleanRoot,
						ChainPosition: -1,
						ParseStatus:   "access_denied",
						DiscoveredAt:  now.UTC().Format(time.RFC3339),
					})
				}
				return nil
			}

			if info.IsDir() {
				if skipDirs[path] {
					return filepath.SkipDir
				}
				return nil
			}

			// Exclude socket/pipe/device
			if !info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
				return nil
			}

			jobs = append(jobs, fileJob{
				path:     path,
				scanRoot: cleanRoot,
			})
			return nil
		})

		if err != nil {
			p := cleanRoot
			rootErrors = append(rootErrors, &ScanError{
				Code:    "walk_error",
				Message: err.Error(),
				Path:    &p,
			})
		}
	}

	// Worker pool for concurrent file inspection
	numWorkers := 8
	if len(jobs) < numWorkers {
		numWorkers = len(jobs)
	}
	if numWorkers < 1 {
		numWorkers = 1
	}

	jobsCh := make(chan fileJob, len(jobs))
	for _, j := range jobs {
		jobsCh <- j
	}
	close(jobsCh)

	resultsCh := make(chan struct {
		obs []*CertObservation
		err *ScanError
	}, len(jobs))

	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobsCh {
				obs, scanErr := inspectFile(job.path, job.scanRoot, now)
				resultsCh <- struct {
					obs []*CertObservation
					err *ScanError
				}{obs: obs, err: scanErr}
			}
		}()
	}

	wg.Wait()
	close(resultsCh)

	allObs := append([]*CertObservation{}, rootObservations...)
	allErrors := append([]*ScanError{}, rootErrors...)

	for res := range resultsCh {
		if len(res.obs) > 0 {
			allObs = append(allObs, res.obs...)
		}
		if res.err != nil {
			allErrors = append(allErrors, res.err)
		}
	}

	// Sort observations deterministically by Path then ChainPosition
	sort.Slice(allObs, func(i, j int) bool {
		if allObs[i].Path == allObs[j].Path {
			return allObs[i].ChainPosition < allObs[j].ChainPosition
		}
		return allObs[i].Path < allObs[j].Path
	})

	// Sort errors deterministically by Path then Code
	sort.Slice(allErrors, func(i, j int) bool {
		pathI := ""
		if allErrors[i].Path != nil {
			pathI = *allErrors[i].Path
		}
		pathJ := ""
		if allErrors[j].Path != nil {
			pathJ = *allErrors[j].Path
		}
		if pathI == pathJ {
			return allErrors[i].Code < allErrors[j].Code
		}
		return pathI < pathJ
	})

	return &ScanResult{
		Observations: allObs,
		Errors:       allErrors,
	}, nil
}

func inspectFile(path, scanRoot string, now time.Time) ([]*CertObservation, *ScanError) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsPermission(err) {
			p := path
			return []*CertObservation{{
				Path:          path,
				Filename:      filepath.Base(path),
				Format:        "UNKNOWN",
				ScanRoot:      scanRoot,
				ChainPosition: -1,
				ParseStatus:   "access_denied",
				DiscoveredAt:  now.UTC().Format(time.RFC3339),
			}}, &ScanError{Code: "access_denied", Message: err.Error(), Path: &p}
		}
		return nil, nil
	}

	isSymlink := info.Mode()&os.ModeSymlink != 0
	var resolvedPath *string
	if isSymlink {
		target, err := os.Readlink(path)
		if err == nil {
			resolvedPath = &target
		}
	} else {
		resolvedPath = &path
	}

	// Skip files over 1 MB limit
	if info.Size() > maxFileSize {
		return nil, nil
	}

	fileMode := fmt.Sprintf("%04o", info.Mode().Perm())
	fileSize := info.Size()
	fileName := filepath.Base(path)
	ext := strings.ToLower(filepath.Ext(path))

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsPermission(err) {
			p := path
			return []*CertObservation{{
				Path:          path,
				ResolvedPath:  resolvedPath,
				Filename:      fileName,
				Format:        "UNKNOWN",
				ScanRoot:      scanRoot,
				ChainPosition: -1,
				FileMode:      fileMode,
				FileSize:      fileSize,
				IsSymlink:     isSymlink,
				ParseStatus:   "access_denied",
				DiscoveredAt:  now.UTC().Format(time.RFC3339),
			}}, &ScanError{Code: "access_denied", Message: err.Error(), Path: &p}
		}

		errMsg := err.Error()
		return []*CertObservation{{
			Path:          path,
			ResolvedPath:  resolvedPath,
			Filename:      fileName,
			Format:        "UNKNOWN",
			ScanRoot:      scanRoot,
			ChainPosition: -1,
			FileMode:      fileMode,
			FileSize:      fileSize,
			IsSymlink:     isSymlink,
			ParseStatus:   "parse_failed",
			ParseError:    &errMsg,
			DiscoveredAt:  now.UTC().Format(time.RFC3339),
		}}, nil
	}

	// Sniff format
	isPEM := bytes.Contains(data, []byte("-----BEGIN "))
	isDER := len(data) >= 2 && data[0] == 0x30 && (data[1] == 0x82 || (data[1]&0x80 == 0))
	isPKCS12 := ext == ".p12" || ext == ".pfx"

	isCertExt := ext == ".pem" || ext == ".crt" || ext == ".cer" || ext == ".der" || isPKCS12

	if !isCertExt && !isPEM && !isDER {
		// Not a certificate file, ignore
		return nil, nil
	}

	owner := attribution.FindFileOwner(path)
	discoveredAt := now.UTC().Format(time.RFC3339)

	// Case 1: PKCS12 / PFX
	if isPKCS12 {
		return inspectPKCS12(path, resolvedPath, fileName, scanRoot, fileMode, fileSize, isSymlink, data, owner, discoveredAt, now)
	}

	// Case 2: PEM
	if isPEM || ext == ".pem" {
		return inspectPEM(path, resolvedPath, fileName, scanRoot, fileMode, fileSize, isSymlink, data, owner, discoveredAt, now)
	}

	// Case 3: DER (or .crt / .cer containing DER binary)
	if isDER || ext == ".der" || ext == ".crt" || ext == ".cer" {
		return inspectDER(path, resolvedPath, fileName, scanRoot, fileMode, fileSize, isSymlink, data, owner, discoveredAt, now)
	}

	return nil, nil
}

func inspectPEM(path string, resolvedPath *string, fileName, scanRoot, fileMode string, fileSize int64, isSymlink bool, data []byte, owner *attribution.Owner, discoveredAt string, now time.Time) ([]*CertObservation, *ScanError) {
	var certBlocks [][]byte
	hasPrivKey := false
	rest := data

	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if strings.Contains(block.Type, "PRIVATE KEY") {
			hasPrivKey = true
			// Never retain or log private key material
			continue
		}
		if block.Type == "CERTIFICATE" {
			certBlocks = append(certBlocks, block.Bytes)
		}
	}

	// If no certificate blocks were found
	if len(certBlocks) == 0 {
		parseStatus := "parse_failed"
		var parseErr *string
		if hasPrivKey {
			msg := "file contains private key but no certificate"
			parseErr = &msg
		} else {
			msg := "no certificate found in PEM"
			parseErr = &msg
		}

		return []*CertObservation{{
			Path:               path,
			ResolvedPath:       resolvedPath,
			Filename:           fileName,
			Format:             "PEM",
			ScanRoot:           scanRoot,
			ChainPosition:      -1,
			ChainLength:        0,
			FileMode:           fileMode,
			FileSize:           fileSize,
			IsSymlink:          isSymlink,
			ContainsPrivateKey: hasPrivKey,
			ParseStatus:        parseStatus,
			ParseError:         parseErr,
			Owner:              owner,
			DiscoveredAt:       discoveredAt,
		}}, nil
	}

	var results []*CertObservation
	chainLen := len(certBlocks)

	for i, derBytes := range certBlocks {
		cert, err := x509.ParseCertificate(derBytes)
		if err != nil {
			msg := fmt.Sprintf("x509 parse certificate error: %v", err)
			results = append(results, &CertObservation{
				Path:               path,
				ResolvedPath:       resolvedPath,
				Filename:           fileName,
				Format:             "PEM",
				ScanRoot:           scanRoot,
				ChainPosition:      i,
				ChainLength:        chainLen,
				FileMode:           fileMode,
				FileSize:           fileSize,
				IsSymlink:          isSymlink,
				ContainsPrivateKey: hasPrivKey,
				ParseStatus:        "parse_failed",
				ParseError:         &msg,
				Owner:              owner,
				DiscoveredAt:       discoveredAt,
			})
			continue
		}

		days, validity := certinfo.ExpiryInfo(cert, now)
		cModel := certinfo.ExtractCertificate(cert)

		results = append(results, &CertObservation{
			Path:               path,
			ResolvedPath:       resolvedPath,
			Filename:           fileName,
			Format:             "PEM",
			ScanRoot:           scanRoot,
			ChainPosition:      i,
			ChainLength:        chainLen,
			FileMode:           fileMode,
			FileSize:           fileSize,
			IsSymlink:          isSymlink,
			ContainsPrivateKey: hasPrivKey,
			ParseStatus:        "ok",
			DaysToExpiry:       &days,
			ValidityStatus:     &validity,
			Owner:              owner,
			DiscoveredAt:       discoveredAt,
			Certificate:        cModel,
		})
	}

	return results, nil
}

func inspectPKCS12(path string, resolvedPath *string, fileName, scanRoot, fileMode string, fileSize int64, isSymlink bool, data []byte, owner *attribution.Owner, discoveredAt string, now time.Time) ([]*CertObservation, *ScanError) {
	// Try empty password first per docs/02
	privKey, leafCert, caCerts, err := pkcs12.DecodeChain(data, "")
	hasPrivKey := privKey != nil
	privKey = nil // Discard private key material immediately

	if err != nil {
		// Try decode trust store
		trustCerts, errTrust := pkcs12.DecodeTrustStore(data, "")
		if errTrust == nil && len(trustCerts) > 0 {
			var results []*CertObservation
			chainLen := len(trustCerts)
			for i, cert := range trustCerts {
				days, validity := certinfo.ExpiryInfo(cert, now)
				cModel := certinfo.ExtractCertificate(cert)
				results = append(results, &CertObservation{
					Path:               path,
					ResolvedPath:       resolvedPath,
					Filename:           fileName,
					Format:             "PKCS12",
					ScanRoot:           scanRoot,
					ChainPosition:      i,
					ChainLength:        chainLen,
					FileMode:           fileMode,
					FileSize:           fileSize,
					IsSymlink:          isSymlink,
					ContainsPrivateKey: false,
					ParseStatus:        "ok",
					DaysToExpiry:       &days,
					ValidityStatus:     &validity,
					Owner:              owner,
					DiscoveredAt:       discoveredAt,
					Certificate:        cModel,
				})
			}
			return results, nil
		}

		parseStatus := "password_protected"
		var parseError *string

		if !errors.Is(err, pkcs12.ErrIncorrectPassword) && !errors.Is(err, pkcs12.ErrDecryption) && !errors.Is(errTrust, pkcs12.ErrIncorrectPassword) && !errors.Is(errTrust, pkcs12.ErrDecryption) {
			// Malformed or corrupt PKCS12
			parseStatus = "parse_failed"
			msg := err.Error()
			parseError = &msg
		}

		return []*CertObservation{{
			Path:               path,
			ResolvedPath:       resolvedPath,
			Filename:           fileName,
			Format:             "PKCS12",
			ScanRoot:           scanRoot,
			ChainPosition:      -1,
			ChainLength:        0,
			FileMode:           fileMode,
			FileSize:           fileSize,
			IsSymlink:          isSymlink,
			ContainsPrivateKey: false,
			ParseStatus:        parseStatus,
			ParseError:         parseError,
			Owner:              owner,
			DiscoveredAt:       discoveredAt,
		}}, nil
	}

	var allCerts []*x509.Certificate
	if leafCert != nil {
		allCerts = append(allCerts, leafCert)
	}
	allCerts = append(allCerts, caCerts...)

	if len(allCerts) == 0 {
		msg := "no certificates found in PKCS12 file"
		return []*CertObservation{{
			Path:               path,
			ResolvedPath:       resolvedPath,
			Filename:           fileName,
			Format:             "PKCS12",
			ScanRoot:           scanRoot,
			ChainPosition:      -1,
			ChainLength:        0,
			FileMode:           fileMode,
			FileSize:           fileSize,
			IsSymlink:          isSymlink,
			ContainsPrivateKey: hasPrivKey,
			ParseStatus:        "parse_failed",
			ParseError:         &msg,
			Owner:              owner,
			DiscoveredAt:       discoveredAt,
		}}, nil
	}

	var results []*CertObservation
	chainLen := len(allCerts)

	for i, cert := range allCerts {
		days, validity := certinfo.ExpiryInfo(cert, now)
		cModel := certinfo.ExtractCertificate(cert)
		results = append(results, &CertObservation{
			Path:               path,
			ResolvedPath:       resolvedPath,
			Filename:           fileName,
			Format:             "PKCS12",
			ScanRoot:           scanRoot,
			ChainPosition:      i,
			ChainLength:        chainLen,
			FileMode:           fileMode,
			FileSize:           fileSize,
			IsSymlink:          isSymlink,
			ContainsPrivateKey: hasPrivKey,
			ParseStatus:        "ok",
			DaysToExpiry:       &days,
			ValidityStatus:     &validity,
			Owner:              owner,
			DiscoveredAt:       discoveredAt,
			Certificate:        cModel,
		})
	}

	return results, nil
}

func inspectDER(path string, resolvedPath *string, fileName, scanRoot, fileMode string, fileSize int64, isSymlink bool, data []byte, owner *attribution.Owner, discoveredAt string, now time.Time) ([]*CertObservation, *ScanError) {
	certs, err := x509.ParseCertificates(data)
	if err != nil {
		cert, singleErr := x509.ParseCertificate(data)
		if singleErr == nil {
			certs = []*x509.Certificate{cert}
		} else {
			msg := fmt.Sprintf("x509 parse DER error: %v", err)
			return []*CertObservation{{
				Path:          path,
				ResolvedPath:  resolvedPath,
				Filename:      fileName,
				Format:        "DER",
				ScanRoot:      scanRoot,
				ChainPosition: -1,
				ChainLength:   0,
				FileMode:      fileMode,
				FileSize:      fileSize,
				IsSymlink:     isSymlink,
				ParseStatus:   "parse_failed",
				ParseError:    &msg,
				Owner:         owner,
				DiscoveredAt:  discoveredAt,
			}}, nil
		}
	}

	if len(certs) == 0 {
		msg := "no certificates found in DER"
		return []*CertObservation{{
			Path:          path,
			ResolvedPath:  resolvedPath,
			Filename:      fileName,
			Format:        "DER",
			ScanRoot:      scanRoot,
			ChainPosition: -1,
			ChainLength:   0,
			FileMode:      fileMode,
			FileSize:      fileSize,
			IsSymlink:     isSymlink,
			ParseStatus:   "parse_failed",
			ParseError:    &msg,
			Owner:         owner,
			DiscoveredAt:  discoveredAt,
		}}, nil
	}

	var results []*CertObservation
	chainLen := len(certs)

	for i, cert := range certs {
		days, validity := certinfo.ExpiryInfo(cert, now)
		cModel := certinfo.ExtractCertificate(cert)
		results = append(results, &CertObservation{
			Path:          path,
			ResolvedPath:  resolvedPath,
			Filename:      fileName,
			Format:        "DER",
			ScanRoot:      scanRoot,
			ChainPosition: i,
			ChainLength:   chainLen,
			FileMode:      fileMode,
			FileSize:      fileSize,
			IsSymlink:     isSymlink,
			ParseStatus:   "ok",
			DaysToExpiry:  &days,
			ValidityStatus: &validity,
			Owner:         owner,
			DiscoveredAt:  discoveredAt,
			Certificate:   cModel,
		})
	}

	return results, nil
}
