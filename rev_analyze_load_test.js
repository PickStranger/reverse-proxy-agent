import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';

const errorRate = new Rate('errors');
const analyzeDuration = new Trend('analyze_duration');

const BASE_URL = 'http://20.89.171.1:9000';

export const options = {
  stages: [
    { duration: '30s', target: 20 },   // 워밍업: 0 → 20명
    { duration: '1m', target: 20 },    // 20명 유지
    { duration: '30s', target: 50 },   // 20 → 50명
    { duration: '1m', target: 50 },    // 50명 유지
    { duration: '30s', target: 100 },  // 50 → 100명
    { duration: '1m', target: 100 },   // 100명 유지
    { duration: '30s', target: 0 },    // 종료
  ],
  thresholds: {
    http_req_duration: ['p(95)<1000'], // AI 판정 포함이라 여유 있게 1000ms 기준
    errors: ['rate<0.05'],
  },
};

export default function () {
  // ─── 1단계: "/" 요청으로 fingerprint 수집 + session_token 발급 ───
  const fpRes = http.get(`${BASE_URL}/`);

  const sessionToken = fpRes.headers['X-Session-Token'];

  const fpSuccess = check(fpRes, {
    'fingerprint request succeeded': (r) => r.status === 200,
    'session_token received': () => !!sessionToken,
  });

  if (!sessionToken) {
    errorRate.add(1);
    sleep(1);
    return;
  }

  // ─── 2단계: "/internal/analyze"로 AI 판정 요청 ───
  const userId = `user_${__VU}_${__ITER}`; // 가상 유저별 고유 user_id 생성

  const analyzeRes = http.post(
    `${BASE_URL}/internal/analyze`,
    JSON.stringify({
      session_token: sessionToken,
      user_id: userId,
    }),
    {
      headers: { 'Content-Type': 'application/json' },
    }
  );

  const analyzeSuccess = check(analyzeRes, {
    'analyze status is 200': (r) => r.status === 200,
    'analyze response has risk_score': (r) => {
      try {
        const body = JSON.parse(r.body);
        return typeof body.risk_score === 'number';
      } catch {
        return false;
      }
    },
  });

  errorRate.add(!(fpSuccess && analyzeSuccess));
  analyzeDuration.add(analyzeRes.timings.duration);

  sleep(1);
}
