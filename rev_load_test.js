import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';

// 커스텀 메트릭: 에러율, 지연시간 추적
const errorRate = new Rate('errors');
const proxyDuration = new Trend('proxy_duration');

// ─── 대상 서버 ───────────────────────────────
const BASE_URL = 'http://20.89.171.1:9000';

// ─── 단계적 부하 증가 시나리오 ───────────────
// 100 → 500 → 1000 동시 사용자까지 단계적으로 올림
export const options = {
  stages: [
    { duration: '30s', target: 50 },   // 0 → 50명, 30초간 서서히 증가 (워밍업)
    { duration: '1m', target: 50 },    // 50명 유지, 1분
    { duration: '30s', target: 100 },  // 50 → 100명 증가
    { duration: '1m', target: 100 },   // 100명 유지, 1분
    { duration: '30s', target: 200 },  // 100 → 200명 증가
    { duration: '1m', target: 200 },   // 200명 유지, 1분
    { duration: '30s', target: 0 },    // 종료 (부하 감소)
  ],
  thresholds: {
    // p95 지연시간이 500ms를 넘으면 테스트 실패로 표시 (기준값, 필요시 조정)
    http_req_duration: ['p(95)<500'],
    // 에러율 1% 미만 유지
    errors: ['rate<0.01'],
  },
};

export default function () {
  const res = http.get(`${BASE_URL}/`);

  const success = check(res, {
    'status is 200': (r) => r.status === 200,
    'response time < 1000ms': (r) => r.timings.duration < 1000,
  });

  errorRate.add(!success);
  proxyDuration.add(res.timings.duration);

  sleep(1); // 각 가상 유저가 요청 사이에 1초 대기 (현실적인 트래픽 패턴)
}
