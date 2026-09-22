# KR-0001b 패치 및 포크 제안

권고: 두 scanner의 `=` guard만 제거하는 작은 패치는 채택 검토 대상으로
유지한다. 원격 포크나 제품 적용은 이번 작업에서 실행하지 않는다.
장기 독립 포크를 시작할 근거는 부족하다.

실험은 사용자 승인 범위인 `.scratch/kr0001b-candidate`의 v0.53.0 복사본에서
수행했다. 3,275개 파일 중 두 파일의 3줄씩만 변경했다. 전후 전체 파일 집합의
해시와 두 source 해시를 `candidate-identity.json`에, 실제 diff를 `candidate.diff`에
보존했다. 테스트 후 파일 집합이 그대로임을 다시 확인했다.

| Phase 3 비교 | baseline | candidate |
|---|---:|---:|
| C와 같은 전체 snapshot | 39 / 50 | 43 / 50 |
| bare `=` 입력 4개 | 오류 tree | C와 같은 clean tree |
| 나머지 입력 46개 | 기준 | Go digest 변화 없음 |
| bare `&` 입력 6개 | 오류 유지 | 오류 및 기존 Go digest 유지 |

표는 이 phase의 baseline 실행, candidate 실행, 새 C 실행만으로 구성했다.
`comparison.json`이 50개 입력의 source/build/세 tree digest를 연결한다.
각 variant를 입력당 한 번 실행했으며 성능 주장은 하지 않는다.

추가 검사:

- F4/F5를 만드는 네 증분 편집 결과가 각각 C fresh snapshot과 같음(E5).
- 후보의 기존 7개 언어 smoke 증분 self-consistency와 bare `&` characterization 통과.
- upstream의 두 scanner binding 테스트와 TSX attribute regression 테스트 통과.
- 제품 비교 테스트가 candidate `replace`를 의도대로 거부함.
- 일반 제품 모듈로 다시 실행한 C 비교와 기존 KR 테스트 통과.

관찰 범위에서 패치의 효과는 bare `=` 오류 네 개에 한정된다. 이는 모든 JSX,
모든 증분 편집, query 동작이나 release 승인을 뜻하지 않는다. C# KR-0002는
그대로 남아 있으며, 이 패치로 해결된다고 주장하지 않는다.

실제 채택을 진행할 때의 제안:

1. 공개 issue #1242에 보낼 재현·C 비교·수정 diff를 먼저 검토한다. 이 문서는
   게시하지 않았고 maintainer와의 연락도 하지 않았다.
2. upstream release를 기다리기 어려운 제품 일정이 확정되면, baseline을 보존한
   관리형 최소 패치를 별도 승인으로 채택한다. 제품 module 교체는 현재 범위 밖이다.
3. upstream에서 같은 수정이 release되면 새 버전의 동일 입력·증분·회귀 결과를
   확인하고 승인된 baseline 이동과 함께 패치를 제거한다. 기존 증거는 보존한다.
4. 원격 포크는 배포·공유에 필요한 실제 목적지가 결정될 때 선택한다. 로컬 비교를
   위해 원격 포크부터 만들 필요는 없다. 독립 runtime 개발이나 광범위 수정은 권하지 않는다.

공개 issue가 열려 있고 assignee/연결 PR이 없다는 사실은 “upstream이 추적하지
않는다”는 증거가 아니다. 기존 문서의 세 번째 포크 조건 확정 표현을 바로잡았다.
현재 확인 가능한 issue: https://github.com/odvcencio/gotreesitter/issues/1242.
최신 tag 목록 재확인은 이번 web 조회에서 확보하지 못했으므로 새로운 tagged fix가
절대로 없다는 주장은 하지 않는다.

검토 결과: 제품 경로의 교체 거부를 유지하면서 opt-in 실험 테스트만 후보 경로를
허용한다. 기존 adapter/runtime 구현, baseline, oracle epoch, scope 변경 없음.
남은 범위: 실제 제품 채택, 전체 release corpus, 장시간·동시성 검사, Linux 실행.
