# PR 초안 — Windows 구문 분석 기반과 독립 증거 구축

기존 저장소에는 실행 가능한 모듈과 파서 결과 경계가 없었다. 이번 변경은 고정된
upstream 의존성 위에 결과·진단 모델, 어댑터, import 경계 검사와 Windows 검증을
구축한다. 성공 반환값이나 전체 span만으로 불완전한 트리를 clean으로 노출하지 않는다.

- Phase 0: 실제 빌드 의존성과 기계 판독 신원을 연결하고 버전 불일치 음성 검사를 남겼다.
- Phase 1: 일곱 언어 smoke와 편집 후 자체 일관성 검사를 추가했다(E3).
- Phase 2: bare `&` 특성 검사와 bare `=` 회귀 검사를 분리했다. TSX F4/F5는
  C clean/Go 오류 차이를 기록했다(E5). 패치 제안은 적용하지 않은 파일이다.
- Phase 3: 별도 모듈로 2×2 비교를 12회 측정했다(E4). 원래 HasError 관찰은
  재현되지 않았고, excerpt의 missing node는 별도 검사로 남겼다(E3).
- 사용자 요청에 따라 생성기는 실행 시 선택하고 실제 버전·ABI·해시를 기록한다.
  비공개 문서와 로컬 환경 파일은 Git에서 제외했다.

검증: Windows/amd64 CGO_ENABLED=0 build/vet/test, 의존성 무결성 검사,
Windows/arm64 빌드, Python ABI 경계 검사, Windows native C 진단 비교가 통과했다.
단계별 조건과 결과는 `artifacts/session-05/phase0/`부터 `phase4/`에 분리되어 있다.
원격 CI·race·전체 oracle lane·실제 CLI 재생성은 실행하지 않았다. 실제 리소스 제한,
invariant violation, 파싱 중 취소 등 비결정적 실패 경로도 미검증이다.

## 리뷰 순서

1. `go.mod`, `internal/provenance/`: 실제 의존성과 고정 신원의 연결 및 음성 검사.
2. `syntax/`, `internal/gtsadapter/`, `internal/boundary/`: 결과 우선순위,
   소유권, 오류·missing 전파, 편집 범위 검사, upstream 형식 차단.
3. `known_regressions_test.go`, `tools/native-oracle/`, Phase 2 증거:
   서로 다른 두 원인, E5의 TSX 한정 범위, 미적용 diff, 동적 생성기 정책.
4. `experiments/v052-v053/`, `testdata/newtonsoft/`, Phase 3 증거:
   고정 해시/라이선스, 동일 harness, 반복 횟수, 경로·버전 분리, missing node.
5. Windows workflow, `.gitignore`, 상태표와 Phase 4 회귀 기록.

이 브랜치는 LARGE이며 bootstrap 예외 대상이다. 로컬 세션 게이트 통과 후
`--no-ff` 병합하며 push·원격 생성·태그·런타임 패치는 수행하지 않는다.
후속 작업은 전체 oracle lane과 C# missing-node 진단이다. TSX 포크/패치 허용 여부는
사용자 결정으로 남는다. Development 완료는 Release 승인과 다르다.
