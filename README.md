# PickStranger Reverse Proxy Agent

로그인 요청의 위험도를 판정하는 Go 기반 리버스 프록시입니다.
모든 요청은 핑거프린트만 남기고 통과시키며, 위험도 판정은 로그인 직후에만 수행합니다. AI 분석 서버가 응답하지 않을 때는 Circuit Breaker가 호출을 건너뛰어 서비스 응답을 유지합니다.

- 기간: 2026.03 ~ 2026.08 (6개월)
- 인원: 2인 프로젝트
- 담당: 리버스 프록시 설계, gRPC 연동, 성능 검증 및 병목 분석, 최적화

## 아키텍처
Client ──▶ rev (Go Reverse Proxy :9000)
├── / 핑거프린트 저장 후 Original Service로 통과
├── /internal/analyze Redis 조회 → AI 위험도 판정 (gRPC)
├── /metrics 요청 수, 지연시간, Circuit Breaker 상태
└── Circuit Breaker ──▶ ai-lb (Nginx, round-robin)
├── rba #1 (XGBoost, gRPC :50051)
└── rba #2 (XGBoost, gRPC :50051)

Prometheus (scrape 5s) ──▶ Grafana (:3000)

- 핑거프린트 저장소: Redis (IP, User-Agent 기반, TTL 저장)
- 판정 경로: 로그인 직후 1회만 AI 서버에 gRPC 요청
- 장애 경로: 연속 5회 실패 시 30초간 AI 호출을 건너뛰고 ALLOW로 응답

## 주요 구현

**핑거프린팅 프록시**
IP와 User-Agent 핑거프린트를 Redis에 TTL과 함께 저장합니다. 모든 요청은 통과시키고, 위험도 판정은 로그인 직후에만 수행합니다.

**gRPC 연동**
AI 서버의 실제 proto 기준으로 인터페이스를 맞췄습니다(Method not found 해결). Mock 서버와 실서버를 환경변수로 전환할 수 있게 해서, AI 서버 없이도 독립적으로 테스트할 수 있습니다.

**Circuit Breaker (Fail-Open)**
AI 서버 장애 시에도 서비스가 멈추지 않도록, 연속 5회 실패하면 30초간 AI 호출을 생략하고 ALLOW로 응답합니다. 우회 상태는 메트릭(`rev_circuit_breaker_open`)으로 노출해 즉시 감지할 수 있습니다.

**관측**
Prometheus로 요청 수, 지연시간, 회로 상태를 수집하고 Grafana 대시보드로 시각화합니다.

## 성능 검증 결과

### 1. 병목 발견 (Circuit Breaker 적용 전, 100 VU)

| 지표 | 결과 |
|---|---|
| 요청 에러율 | 29.07% (성공률 70.9%) |
| P95 응답 시간 | 5.03초 (타임아웃 임계치 5초 초과) |
| 병목 위치 | AI 분석 서버 CPU 185% (2 vCPU 중) |
| Go 프록시 CPU | 1% 미만 (유휴) |

원인은 AI 추론 서버였습니다. 요청당 CPU를 약 120ms 사용하는 연산 집약 작업이라, 2 vCPU 호스트의 처리 한계가 약 15건/초로 계산됩니다.

### 2. 로드 밸런싱 효과 검증 (1대 vs 2대)

가설은 "AI 서버 2대로 늘리면 처리량이 늘어난다"였습니다. 5~50 VU를 단계별로 올리며(단계당 45초) 비교했습니다.

| 구성 | 처리 한계 | P95 |
|---|---|---|
| 1대 | 약 15건/초 | 10 VU까지 0.75초 이하, 50 VU 약 4초 |
| 2대 (Nginx grpc_pass) | 약 15건/초 (1대와 동일) | 동일 |

두 인스턴스는 같은 2 vCPU VM을 공유했습니다. 두 인스턴스가 각각 80~84%로 나뉘어 부하를 받았지만 합산 처리량은 늘지 않았습니다. 병목은 인스턴스 수가 아니라 **호스트 CPU 연산 용량**이었습니다.

### 3. 장애 격리 검증 (Circuit Breaker 적용 후)

- AI 서버를 강제로 중단한 상태에서 연속 실패 시 회로가 Closed(0)에서 Open(1)로 전환되는 것을 Grafana에서 확인
- 100 VU 과부하에서 요청의 62%가 Fail-Open으로 처리되어 서비스 응답은 유지
- 패스스루 경로(`/`)는 200 VU 기준 P95 65ms 유지

**트레이드오프**: Fail-Open은 가용성을 우선한 선택입니다. 우회된 요청은 AI 판정을 받지 않으므로, 보안과 가용성 사이의 비용을 수치로 확인하고 문서화했습니다.

## 결론과 다음 단계

- 같은 호스트에서 AI 서버를 늘려도 처리량은 늘지 않으므로, 추론 자체를 최적화하거나 **별도 호스트로 확장**하는 것이 필요합니다.
- 이번 검증은 2 vCPU 한 대에서 진행했으므로, 별도 호스트 구성은 아직 검증하지 않았습니다.

## 실행 방법

```bash
# 환경 준비 (Docker Compose)
docker compose up -d

# Mock AI 서버로 독립 실행
[ ] 환경변수 설정 예시

# 부하 테스트 (k6)
[ ] 스크립트 경로 및 실행 명령
```

| 환경변수 | 설명 |
|---|---|
| `AI_MODE` | `mock` 또는 `real` [ ] 실제 변수명 확인 |
| `REDIS_ADDR` | Redis 주소 [ ] |
| `AI_GRPC_ADDR` | AI 서버(또는 ai-lb) gRPC 주소 [ ] |

## 디렉토리 구조
```text,
reverse-proxy-agent/
├── cmd/
│   └── main.go                  # 진입점: 설정 로드, 핸들러 등록, 서버 시작
├── internal/
│   ├── config/
│   │   └── config.go            # 환경변수 로드 (.env 지원)
│   ├── grpc/
│   │   ├── mock_client.go       # AIClient 인터페이스, PredictInput/Result, Mock 구현
│   │   ├── grpc_client.go       # 실제 AI gRPC 클라이언트 (rba.RBAService)
│   │   └── circuit_breaker.go   # Circuit Breaker (Fail-Open)
│   ├── middleware/
│   │   ├── fingerprint.go       # 핑거프린트 추출·해시·Redis 저장
│   │   ├── session_token.go     # 세션 토큰 생성 (crypto/rand)
│   │   └── metrics.go           # Prometheus 계측 미들웨어
│   ├── metrics/
│   │   └── metrics.go           # Prometheus 메트릭 정의
│   ├── proxy/
│   │   └── injector.go          # (미사용) 프록시 래퍼, main.go와 중복
│   └── redis/
│       └── client.go            # 블랙리스트·핑거프린트 저장 래퍼
├── pkg/gen/                     # protoc 생성 코드 (커밋에 포함)
│   ├── rba/                     # rba_service.proto → AI 서버 연동 (사용 중)
│   ├── env/                     # environment.proto (SDK ↔ rev 설정 계약)
│   ├── environment/             # environment.proto 이전 생성본 (중복)
│   └── anomaly/                 # anomaly.proto (초기 설계, 현재 미사용)
├── proto/
│   ├── rba_service.proto        # AI 서버 계약 (Predict RPC)
│   ├── environment.proto        # RevEnvironmentConfigService (SDK 연동용)
│   └── anomaly.proto            # 초기 Analyze RPC 정의 (Predict로 대체됨)
├── static/
│   └── fingerprint.js           # 브라우저 핑거프린트 스크립트 (현재 빈 파일)
├── prometheus.yml               # Prometheus 스크랩 설정 (rev:9000, 5초)
├── rev_load_test.js             # k6: 패스스루 경로(/) 부하 테스트
├── rev_analyze_load_test.js     # k6: /internal/analyze 부하 테스트
├── Dockerfile                   # 멀티스테이지 빌드, non-root 실행
├── .dockerignore
├── .gitignore                   # .env 제외
├── .env                         # 로컬 설정 (커밋 안 함)
├── go.mod / go.sum
└── README.md
```

##요청 흐름
```text
클라이언트 ──► rev :9000
                │
                ├─ /                 모든 경로(백엔드로 프록시)
                │    1. 핑거프린트 추출 (IP + User-Agent + Accept-Language → SHA-256)
                │    2. 세션 토큰 발급, Redis에 60초 저장  (fp:<token>)
                │    3. 화이트리스트 IP면 통과
                │    4. 블랙리스트(blacklist:<hash>)면 403
                │    5. X-Session-Token 헤더를 붙여 백엔드로 전달
                │
                ├─ /internal/analyze  SDK가 로그인 직후 호출 (POST)
                │    1. 세션 토큰으로 Redis에서 핑거프린트 조회
                │    2. Circuit Breaker → AI gRPC Predict
                │    3. 토큰 삭제, 이상(is_anomaly)이면 핑거프린트 해시를 블랙리스트에 등록
                │    4. {risk_score, is_anomaly, action(ALLOW/CHALLENGE/BLOCK)} 응답
                │
                └─ /metrics           Prometheus 지표
```

## 팀

- 2인 프로젝트 (리버스 프록시 설계, gRPC 연동, 성능 검증 및 병목 분석 담당)
