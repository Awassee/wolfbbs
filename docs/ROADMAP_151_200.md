# WolfBBS Roadmap 151-200

This document extends the ordered backlog beyond `docs/ROADMAP_101_150.md`.

## Delivery rule
- Keep shipping in validated tranches.
- Do not skip ahead in code delivery just because a later item sounds bigger.
- Every tranche ends with tests, verifier pass, packaged artifacts, and a release.

## Theme 16: Caller retention and return loops
151. Daily streaks across boards, chat, and doors.
Value: creates a simple reason to come back tomorrow.
Status: shipped (`/streaks` route with caller streak board and cross-surface activity metrics).

152. Personalized "next best action" cards.
Value: shortens the gap between sign-in and useful activity.
Status: shipped (`/next` route generates caller-specific action cards from unread + engagement state).

153. Featured returning-caller spotlights.
Value: rewards participation and makes the board feel inhabited.
Status: shipped (`/spotlights` route highlights returning callers with streak/activity context).

154. Seasonal missions with completion tracking.
Value: adds medium-term goals beyond one session.
Status: shipped (`/admin/missions` + `/missions` with mission definition, progress computation, and claim persistence).

155. Returning caller digest preferences by weekday.
Value: lets users shape heavier vs lighter attention by routine.
Status: shipped (`/digest/preferences` with weekday item caps applied to digest generation).

156. Smart re-entry to the last unfinished workflow.
Value: preserves momentum between sessions.
Status: shipped (`/resume` route prioritizes unfinished workflows and allows one-click home-route update).

157. Door comeback prompts after streak breaks.
Value: reconnects casual callers to sticky game loops.
Status: shipped (`/doors/comeback` route shows streak-aware comeback prompts and recommended doors).

158. New user mentorship pairing.
Value: improves onboarding quality and retention.
Status: shipped (`/admin/mentorship` + `/mentorship` routes support pair management and direct mentor check-ins).

159. Profile milestone celebrations.
Value: turns progress into visible social proof.
Status: shipped (`/milestones` route computes milestone progress and persists celebration state).

160. Time-of-day tailored landing states.
Value: makes the product feel alive instead of static.
Status: shipped (`/time-lane` route adapts lane guidance by time-of-day and sets home-route preferences).

## Theme 17: Moderation, trust, and operator workflow
161. Moderator assignment queue for reports.
Value: avoids duplicated handling and dropped reports.
Status: shipped (`/admin/mod-center` assignment queue persists report owner/priority/status state).

162. Report SLA timers and breach warnings.
Value: gives staff a real follow-through metric.
Status: shipped (`/admin/mod-center` computes SLA warning/breach states from report due windows).

163. Caller risk summary on staff-visible profiles.
Value: speeds support and moderation judgment.
Status: shipped (`/directory` staff profile view now includes risk score/level/signals for the selected caller).

164. Moderator canned responses with audit trails.
Value: improves consistency and reduces response time.
Status: shipped (`/admin/mod-center` stores reusable canned templates, sends them as mail, and records admin actions).

165. Case threads for multi-step incidents.
Value: groups related actions into one reviewable record.
Status: shipped (`/admin/mod-center` creates, updates, and closes moderation case threads with timeline updates).

166. Abuse trend dashboard by board, user, and channel.
Value: helps sysops prioritize structural fixes.
Status: planned.

167. Staff handoff notes with explicit ownership.
Value: makes unresolved issues survivable across shifts.
Status: planned.

168. Soft quarantine lane for suspicious uploads/posts.
Value: reduces accidental over-deletion.
Status: planned.

169. Moderator digests by role and area.
Value: gives each staff member a filtered work queue.
Status: planned.

170. Reusable moderation rule presets.
Value: reduces policy drift across boards and staff.
Status: planned.

## Theme 18: Files, packets, and offline depth
171. File request board with claimant workflow.
Value: makes requested content visible and actionable.
Status: planned.

172. Upload provenance cards.
Value: improves trust around new files.
Status: planned.

173. File collections with curated descriptions.
Value: turns the filebase into a destination, not just storage.
Status: planned.

174. Download resume tokens.
Value: makes larger file delivery more forgiving.
Status: planned.

175. Bulk upload draft sessions.
Value: improves contributor workflow quality.
Status: planned.

176. Offline board packet subscriptions.
Value: supports callers who read asynchronously.
Status: planned.

177. Offline reply merge conflict handling.
Value: makes packet workflows safe instead of brittle.
Status: planned.

178. Automatic duplicate candidate review for uploads.
Value: lowers curator workload.
Status: planned.

179. Featured file collection landing page.
Value: gives the filebase a browsable front door.
Status: planned.

180. Filebase repair assistant for orphaned metadata.
Value: improves operator recovery and data hygiene.
Status: planned.

## Theme 19: Events, clubs, and community programming
181. RSVP tracking for community events.
Value: lets hosts see actual intent, not just page views.
Status: planned.

182. Event recap pages with stats and highlights.
Value: compounds value from each hosted event.
Status: planned.

183. Clubhouse groups with membership rolls.
Value: adds durable social identity around interests.
Status: planned.

184. Hosted "net" templates for chat and boards.
Value: makes recurring community sessions easier to run.
Status: planned.

185. Board club challenges tied to shared goals.
Value: turns content and doors into linked loops.
Status: planned.

186. Guest-visible event teaser cards.
Value: makes the public surface feel active.
Status: planned.

187. Event attendance rewards.
Value: strengthens participation loops.
Status: planned.

188. Host checklists and event runbooks.
Value: lowers operational friction for community programming.
Status: planned.

189. Cross-route event promotion slots.
Value: keeps events visible without manual duplication.
Status: planned.

190. Event sponsorship or spotlight rotation.
Value: creates a richer editorial rhythm.
Status: planned.

## Theme 20: Platform packaging, extension, and release ops
191. Release dashboard for current branch state.
Value: shortens the path from validation to shipping.
Status: planned.

192. Backup health surface in web admin.
Value: makes recovery posture visible before failure.
Status: planned.

193. Upgrade dry-run preview with change summary.
Value: improves trust in installer-driven upgrades.
Status: planned.

194. Theme import/export bundles.
Value: makes customization portable.
Status: planned.

195. Extension manifest validation tooling.
Value: lowers risk before plugin support broadens.
Status: planned.

196. Webhook subscriptions for board and mail events.
Value: improves integration potential.
Status: planned.

197. Release-note generation from roadmap status.
Value: ties product planning directly to shipment.
Status: planned.

198. Operator analytics snapshot page.
Value: helps prioritize product decisions from actual usage.
Status: planned.

199. Environment diagnostics bundle export.
Value: speeds support and troubleshooting.
Status: planned.

200. Consumer release checklist automation.
Value: makes public shipping repeatable and less error-prone.
Status: planned.
