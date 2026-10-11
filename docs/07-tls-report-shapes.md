# 07 TLS Report Shapes (backend <-> UI). All counts are ints. "NameCount" = {"name":string,"count":int}.

GET /api/v1/runs/{id}/tls/summary
{"events":int,            // SUM(record_count) over run observations
 "connections":int,       // COUNT(*) observations
 "unique_endpoints":int,  // COUNT DISTINCT (remote_ip, remote_port)
 "unique_ports":int,
 "certs_observed":int,    // COUNT DISTINCT peer_cert_fp (non-null)
 "relationships":int,     // COUNT of edge_run_stats rows for this run
 "versions":[NameCount], "ciphers":[NameCount], "kex_groups":[NameCount],
 "directions":[NameCount], "peer_cert_sources":[NameCount], "attribution":[NameCount],
 "top_endpoints":[NameCount],                 // name = "ip:port" or "sni (ip:port)"
 "cert_posture":[NameCount],                  // valid|expiring|expired|unknown, evaluated at query time (expiring <= 30d)
 "timeline":[{"ts":"RFC3339","events":int}]}  // 5-second buckets of first_ts, sum of record_count

GET /api/v1/runs/{id}/tls/observations?group=none|endpoint&q=&version=&cipher=&direction=&attribution=&limit=&offset=
 group=none  -> {"items":[{tls_observations columns..., "peer_subject_cn","peer_issuer_cn","peer_not_after",
                           "peer_pubkey_algo","peer_pubkey_bits","peer_validity"}],"total":int,"limit":int,"offset":int}
 group=endpoint -> {"items":[{"remote_ip","remote_port","sni","connections":int,"events":int,
                              "versions":[string],"ciphers":[string],"processes":[string],"directions":[string]}],"total":int}

GET /api/v1/runs/{id}/tls/evidence
{"events":{"total":int,"formula":"SUM(record_count)","breakdown":{"handshake":int,"appdata":int,"alert":int,"other":int},
           "by_connection":[{"conn_key","remote":"ip:port","sni","record_count":int}]},   // top 200 by record_count
 "unique_endpoints":{"total":int,"formula":"COUNT(DISTINCT remote_ip, remote_port)","items":["ip:port"]},
 "certs_observed":{"total":int,"items":[{"fingerprint_sha256","source":"observed|probed"}]}}

Graph (existing endpoint) node.props for endpoints: {"ip","port","sni"}; process: {"exe","comm","unit"}.
