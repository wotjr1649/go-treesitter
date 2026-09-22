# CGO 의존성과 남은 작업 점검 — 2026-09-22

점검 대상은 `session/06-oracle-lane`의 소스 커밋
`235e97ef5f7030080e2fe3735ef093457075e73a`이다. 제품 코드, 의존성,
문법, oracle epoch, 공개 범위를 바꾸지 않은 독립 점검이다.
이 디렉터리의 결과만으로 아래 CGO 관찰을 뒷받침한다.

현재 `windows/amd64` 제품 빌드와 실행은 CGO 없이 동작한다(E3).
외부 소비자가 사용할 공개 파서 진입점과 전체 출시 검증은 아직 남아 있다.
CGO 독립성과 구문 분석의 정확성, API 완성도는 별개의 판단이다.

## 이번 점검에서 관찰한 내용

환경은 Go 1.27.1, Windows/amd64, 원본 `gotreesitter v0.53.0`이다.
`CC`와 `CXX`는 존재하지 않는 실행 파일을 가리키며, 자식 프로세스의
PATH에는 Go, Git, Windows System32만 들어 있다. 모듈 및 도구 자동
다운로드를 끄고 저장소 안의 기존 모듈 캐시를 사용했다.
각 검사는 최초 한 번 실행하고 명령당 제한은 180초로 설정했다.
환경과 입력 해시는 [environment.json](environment.json), 명령과 종료 코드는
[commands.jsonl](commands.jsonl)에 있다.

| 검사 | 관찰 | 근거 |
|---|---|---|
| `go list -deps -json ./...`, CGO=0 | 137개 패키지. CgoFiles/CFiles/CXXFiles/MFiles/FFiles/SwigFiles/SwigCXXFiles/SysoFiles 모두 없음. runtime/cgo 없음. | [의존성 요약](dependency-summary-cgo0.json) |
| 같은 의존성 조사, CGO=1 | 같은 137개 패키지, 같은 결과. 이 행은 목록 조사이며 CGO=1 빌드·실행 검사가 아니다. | [의존성 요약](dependency-summary-cgo1.json) |
| 선택된 비표준 Go 소스 758개 검사 | C 호출, 외부 명령 실행, DLL 로딩 경로 검색에서 관련 코드 없음. 유일한 문자열 일치는 언어 이름 `"C"`이다. | [검색 결과](selected-source-scan.json) |
| `CGO_ENABLED=0 go build -a ./...` | 기존 빌드 캐시의 최신 여부와 관계없이 다시 컴파일, exit 0. | [빌드](force-build.log) |
| `go vet ./...` | exit 0. | [vet](vet.log) |
| `go test ./... -count=1 -timeout=120s -json` | 4개 패키지, 상위 테스트 13개 통과. 하위 테스트를 포함한 pass 이벤트 90개, fail 0, skip 0. | [테스트](tests.log), [집계](results.json) |
| `go mod verify` | 모듈 무결성 검사 exit 0. 실제 해석된 의존성에 replace 없음. | [무결성](module-integrity.log), [의존성](resolved-module.log) |
| 파서 테스트 실행 파일과 일반 build-info 실행 파일 | 둘 다 링크 성공. 일반 실행 파일은 실행 성공, 빌드 정보에 CGO_ENABLED=0 및 v0.53.0 기록. | [빌드 정보](binary-build-settings.log) |
| Windows/arm64 일반 실행 파일 | CGO=0 링크 성공. ARM64 실행은 NOT_RUN. | [링크](arm64-link.log) |
| 세 Windows 실행 파일의 PE import table | `kernel32.dll`만 존재. C parser DLL이나 C runtime DLL을 직접 import하지 않음. 런타임의 모든 지연 DLL 로딩을 열거한 결과는 아니다. | [PE 검사](pe-imports.log) |

전체 테스트에는 7개 언어 경로의 fresh/incremental 검사와 현재 오류 형태를
고정한 회귀 검사가 포함된다. 알려진 결함이 고쳐졌다는 뜻은 아니다.
컴파일러를 사용할 수 없는 위 환경에서도 제품 테스트가 저장된 C 기록을
읽어 실행됐으며, 이번 점검에서는 C oracle 실행 파일을 생성하거나 실행하지 않았다.

빌드·실행 주장은 저장소의 기존 테스트 및 이번 실행으로 E3 범위에 한정한다.
추가 의존성/DLL/외부 소비자 탐침은 E1/E2 보강 자료다. E6 주장은 없다.
일반 실행 파일의 `vcs.modified=true`는 점검 중 생성한 이 증거 디렉터리가
미추적 상태였기 때문이다. 당시 기존 추적 파일의 diff는 비어 있었다.
`go.mod`, `go.sum`, `identities.json`의 전후 해시는 동일하다.

## 외부 소비자 API 점검

별도 모듈 `example.com/cgo-audit-consumer`에서 로컬 저장소를 의존성으로
사용했다. `syntax` 타입을 가져온 프로그램은 빌드·실행됐다. 이 프로그램은
파서를 호출하지 않았고, 이 성공을 파서 통합 성공으로 해석하지 않는다.

실제 구현 `internal/gtsadapter.Adapter`를 가져오는 프로그램은
`use of internal package ... not allowed`로 빌드가 거부됐다.
첫 시도에는 별도 모듈의 go.sum 누락 오류도 함께 있었으므로,
원본 모듈의 체크섬과 동일 버전의 간접 의존성을 별도 소비자 모듈에
명시한 뒤 한 번 더 검사했다. 두 번째 시도는 internal 접근 제한만으로
거부됐다. 제품이나 접근 제한은 변경하지 않았다.

- [공개 타입 실행](consumer-types-execute.log)
- [최초 소비자 진단](consumer-parser-build.log)
- [체크섬 보완 후 단일 원인 진단](consumer-parser-complete-checksums.log)
- [공개 타입 목록](public-api.log): 오프라인 모듈 탐색 경고도 원문 보존.

따라서 다음 통합 작업은 공개 생성 진입점을 정하고 외부 모듈에서
생성 → 파싱 → 오류 처리 → 증분 편집 → Close까지 실행하는 것이다.
현재 공개 API에는 이 저장소의 파서 구현을 얻는 함수가 없다.

## 남은 작업의 우선순위

이 표는 현재 계약·상태 문서와 API 관찰을 정리한 작업 목록이다.
과거 실험을 이번 CGO 증거와 합쳐 새 정확성 주장을 만들지 않는다.

| 순서 | 작업 | 남은 이유 |
|---|---|---|
| 1 | KR-0001b 제품 해결, KR-0002 원인 규명·해결 | JSX/TSX의 `=` 패치 후보는 격리 실험에만 존재한다. C# recovery 차이도 열려 있다. 제품 적용은 이번 점검 범위가 아니다. bare `&`의 KR-0001a는 오류로 유지해야 한다. |
| 2 | 공개 파서 진입점과 외부 소비자 통합 | 현재 외부에 타입/인터페이스만 노출되며 구현은 internal이다. Integration gate를 끝내려면 실제 소비자 경로가 필요하다. |
| 3 | 7개 경로의 더 넓은 fresh·incremental C 비교 | 현재 50개 입력은 전체 Release-Critical corpus가 아니다. 복합 편집, UTF-8 경계, 오류 복구 등을 확대해야 한다. |
| 4 | 자원·동시성·수명 검증 | 시간·메모리 예산, 큰 입력, 취소, 반복 실행, 독립 worker와 Tree 소유권 검증이 남아 있다. race는 별도 진단 경로다. |
| 5 | 플랫폼 실행·패키징·출시 절차 | ARM64 실제 실행, 원격 Windows CI 실행, 배포 패키지·라이선스·사용 예제·제한 목록 및 E6 gate가 남아 있다. 다른 OS는 첫 출시 범위 밖이다. |
| 6 | 운영 및 소비자 요구 후 확장 | upstream 추적 절차와 패치 회수 조건을 운영해야 한다. query/tags/outline 검증은 실제 소비자 요구가 생길 때 진행한다. |

관련 현재 문서: [상태 보드](../../docs/reports/release-critical-status.md),
[회귀 등록부](../../docs/validation/known-regressions.md),
[검증 계약](../../docs/specs/validation.md),
[구조](../../docs/design/overview.md).

생성기 버전 고정은 추가하지 않았다. 기존 도구는 호출 시 선택된 생성기와
생성 결과의 정체성을 기록한다. C compiler와 generator는 C 비교 기록을
새로 만드는 개발 도구이며, 제품 사용 및 저장된 기록을 읽는 테스트의 의존성이 아니다.

## 검토와 범위

검토 기준은 `docs/reviews/review-checklist.md`이다. 이번 변경은 점검 기록뿐이며
기존 제품 코드·테스트·계약·CI에는 변경이 없다. 테스트의 예상 오류,
체크섬 진단, 오프라인 경고를 숨기지 않았다. 초기 보조 JSON 읽기는 Windows
기본 cp949 디코딩 오류로 중단되어 UTF-8을 명시해 다시 읽었다. 제품 검사 실패는 아니다.

NOT_RUN: ARM64 실행, Linux/macOS/WASM 실행, 원격 CI, race 진단, 새로운
C 기록 생성, 전체 출시 corpus 및 자원 캠페인. 각각 이번 CGO 점검의
범위 밖이며 실행 성공으로 대신 기록하지 않는다.

`session/06-oracle-lane`은 ADR-0005의 LARGE 분류로 그대로 둔다.
`main`은 `2f809b5f855cced1c17420636136f7744173a3d0` 그대로다.
원격 작업, 제품 패치, baseline/epoch 변경은 없다. 원본 한글 인계는
Git에서 제외한 `artifacts/handoff/`에 별도로 기록한다.
