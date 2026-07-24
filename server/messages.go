package server

// Server error bodies used to be English only, while the web frontend has
// shipped English, Chinese and Japanese for as long as it has existed. A
// visitor reading the site in Japanese still got "Request Entity Too Large"
// when an upload was refused, and the command-line client — which surfaces the
// server's body verbatim — had no language at all.
//
// The catalogue is deliberately small and flat: these are the fixed sentences
// the server chooses to show a user. Errors that carry internal detail
// (storage failures, template errors, ClamAV transport problems) stay in
// English on purpose — they are for whoever reads the logs, and translating
// them would only make the same text harder to search for.

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

type msgID string

const (
	msgNotFound         msgID = "not_found"
	msgInternalError    msgID = "internal_error"
	msgForbidden        msgID = "forbidden"
	msgTooManyRequests  msgID = "too_many_requests"
	msgEntityTooLarge   msgID = "entity_too_large"
	msgNotAuthorized    msgID = "not_authorized"
	msgFileNotFound     msgID = "file_not_found"
	msgRetrieveFailed   msgID = "retrieve_failed"
	msgNoValidFiles     msgID = "no_valid_files"
	msgCollectionBody   msgID = "collection_body"
	msgCollectionEmpty  msgID = "collection_empty"
	msgCollectionFull   msgID = "collection_full"
	msgNotAShareLink    msgID = "not_a_share_link"
	msgFileUnavailable  msgID = "file_unavailable"
	msgCollectionFailed msgID = "collection_failed"
	msgCollectionDelete msgID = "collection_delete"
	msgDecryptFailed    msgID = "decrypt_failed"
	msgCopyFailed       msgID = "copy_failed"
	msgDeleteFailed     msgID = "delete_failed"
	msgQRMissingURL     msgID = "qr_missing_url"
	msgQRForeignURL     msgID = "qr_foreign_url"
	msgQRRenderFailed   msgID = "qr_render_failed"
	msgOwnerTokenShort  msgID = "owner_token_short"
	msgOwnerListFailed  msgID = "owner_list_failed"
	msgServerFull       msgID = "server_full"
	msgSpoolFull        msgID = "spool_full"
	msgUploadLength     msgID = "upload_length"
	msgResumableNoPass  msgID = "resumable_no_password"
	msgSessionFailed    msgID = "session_failed"
	msgSessionExpired   msgID = "session_expired"
	msgSessionMismatch  msgID = "session_mismatch"
	msgSessionContinue  msgID = "session_continue"
	msgSessionOffset    msgID = "session_offset"
	msgChunkIncomplete  msgID = "chunk_incomplete"
	msgSaveFailed       msgID = "save_failed"
	msgMultipartFailed  msgID = "multipart_failed"
	msgReadUpload       msgID = "read_upload"
	msgBufferFailed     msgID = "buffer_failed"
	msgResetCache       msgID = "reset_cache"
	msgEmptyUpload      msgID = "empty_upload"
	msgPrescanFailed    msgID = "prescan_failed"
	msgVirusFound       msgID = "virus_found"
	msgNoVirusTotal     msgID = "no_virustotal"
	msgMaxDownloads     msgID = "max_downloads"
	msgMaxDays          msgID = "max_days"
	msgMaxDaysTooLarge  msgID = "max_days_too_large"
	msgContentRangeForm msgID = "content_range_form"
	msgContentRangeFrom msgID = "content_range_from"
	msgContentRangeTo   msgID = "content_range_to"
	msgContentRangeAll  msgID = "content_range_all"
	msgContentRangeSpan msgID = "content_range_span"
)

// defaultLang is what an unrecognised or absent Accept-Language gets, and the
// language every message is guaranteed to have.
const defaultLang = "en"

// messages holds one entry per user-facing sentence. A language missing an
// entry falls back to English rather than to the message ID.
var messages = map[msgID]map[string]string{
	msgNotFound: {
		"en": "Not Found",
		"zh": "未找到",
		"ja": "見つかりません",
	},
	msgInternalError: {
		"en": "Internal Server Error",
		"zh": "服务器内部错误",
		"ja": "サーバー内部エラー",
	},
	msgForbidden: {
		"en": "Forbidden",
		"zh": "禁止访问",
		"ja": "アクセスが拒否されました",
	},
	msgTooManyRequests: {
		"en": "Too Many Requests",
		"zh": "请求过于频繁",
		"ja": "リクエストが多すぎます",
	},
	msgEntityTooLarge: {
		"en": "Request Entity Too Large",
		"zh": "上传内容超过大小上限",
		"ja": "アップロードのサイズ上限を超えています",
	},
	msgNotAuthorized: {
		"en": "Not authorized",
		"zh": "未授权",
		"ja": "認証が必要です",
	},
	msgFileNotFound: {
		"en": "File not found",
		"zh": "文件不存在",
		"ja": "ファイルが見つかりません",
	},
	msgRetrieveFailed: {
		"en": "Could not retrieve file.",
		"zh": "无法读取文件。",
		"ja": "ファイルを取得できませんでした。",
	},
	msgNoValidFiles: {
		"en": "No valid files requested",
		"zh": "请求中没有有效的文件",
		"ja": "有効なファイルが指定されていません",
	},
	msgCollectionBody: {
		"en": `Body must be JSON: {"files": ["<url>", …]}`,
		"zh": `请求体必须是 JSON：{"files": ["<url>", …]}`,
		"ja": `リクエスト本文は JSON である必要があります：{"files": ["<url>", …]}`,
	},
	msgCollectionEmpty: {
		"en": "A collection needs at least one file",
		"zh": "集合至少需要一个文件",
		"ja": "コレクションには少なくとも 1 つのファイルが必要です",
	},
	msgCollectionFull: {
		"en": "A collection holds at most %d files",
		"zh": "一个集合最多包含 %d 个文件",
		"ja": "コレクションに含められるファイルは最大 %d 件です",
	},
	msgNotAShareLink: {
		"en": "%q is not a share link from this server",
		"zh": "%q 不是本服务器的分享链接",
		"ja": "%q はこのサーバーの共有リンクではありません",
	},
	msgFileUnavailable: {
		"en": "%s is not available",
		"zh": "%s 已不可用",
		"ja": "%s は利用できません",
	},
	msgCollectionFailed: {
		"en": "Could not create the collection",
		"zh": "无法创建集合",
		"ja": "コレクションを作成できませんでした",
	},
	msgCollectionDelete: {
		"en": "Could not delete the collection",
		"zh": "无法删除集合",
		"ja": "コレクションを削除できませんでした",
	},
	msgDecryptFailed: {
		"en": "Could not decrypt file",
		"zh": "无法解密文件",
		"ja": "ファイルを復号できませんでした",
	},
	msgCopyFailed: {
		"en": "Error occurred copying to output stream",
		"zh": "写入响应流时出错",
		"ja": "レスポンスへの書き込み中にエラーが発生しました",
	},
	msgDeleteFailed: {
		"en": "Could not delete file.",
		"zh": "无法删除文件。",
		"ja": "ファイルを削除できませんでした。",
	},
	msgQRMissingURL: {
		"en": "missing url parameter",
		"zh": "缺少 url 参数",
		"ja": "url パラメータがありません",
	},
	msgQRForeignURL: {
		"en": "url must point at this server",
		"zh": "url 必须指向本服务器",
		"ja": "url はこのサーバーを指している必要があります",
	},
	msgQRRenderFailed: {
		"en": "could not render QR code",
		"zh": "无法生成二维码",
		"ja": "QR コードを生成できませんでした",
	},
	msgOwnerTokenShort: {
		"en": "X-Owner-Token must be at least %d characters",
		"zh": "X-Owner-Token 至少需要 %d 个字符",
		"ja": "X-Owner-Token は %d 文字以上である必要があります",
	},
	msgOwnerListFailed: {
		"en": "Could not read the upload list",
		"zh": "无法读取上传列表",
		"ja": "アップロード一覧を読み取れませんでした",
	},
	msgServerFull: {
		"en": "This server is full",
		"zh": "服务器存储空间已满",
		"ja": "このサーバーの保存容量がいっぱいです",
	},
	msgSpoolFull: {
		"en": "This server has no room for another upload in progress",
		"zh": "服务器暂时没有空间容纳新的上传",
		"ja": "進行中のアップロードを追加する空き容量がありません",
	},
	msgUploadLength: {
		"en": "Upload-Length must be the total size in bytes",
		"zh": "Upload-Length 必须是以字节为单位的总大小",
		"ja": "Upload-Length はバイト単位の合計サイズである必要があります",
	},
	msgResumableNoPass: {
		"en": "X-Encrypt-Password is not available on resumable uploads — encrypt on the client instead",
		"zh": "断点续传上传不支持 X-Encrypt-Password，请改为在客户端加密",
		"ja": "レジューム可能なアップロードでは X-Encrypt-Password は使えません。クライアント側で暗号化してください",
	},
	msgSessionFailed: {
		"en": "Could not create upload session",
		"zh": "无法创建上传会话",
		"ja": "アップロードセッションを作成できませんでした",
	},
	msgSessionExpired: {
		"en": "This upload session has expired",
		"zh": "该上传会话已过期",
		"ja": "このアップロードセッションは期限切れです",
	},
	msgSessionMismatch: {
		"en": "Content-Range total does not match the size this session was created for",
		"zh": "Content-Range 中的总大小与该会话创建时声明的大小不一致",
		"ja": "Content-Range の合計サイズがセッション作成時のサイズと一致しません",
	},
	msgSessionContinue: {
		"en": "Could not continue upload",
		"zh": "无法继续上传",
		"ja": "アップロードを継続できませんでした",
	},
	msgSessionOffset: {
		"en": "This session is at byte %d",
		"zh": "该会话当前位于第 %d 字节",
		"ja": "このセッションは %d バイト目まで受信しています",
	},
	msgChunkIncomplete: {
		"en": "Chunk did not arrive in full",
		"zh": "分片未完整送达",
		"ja": "チャンクが完全に届きませんでした",
	},
	msgSaveFailed: {
		"en": "Could not save file",
		"zh": "无法保存文件",
		"ja": "ファイルを保存できませんでした",
	},
	msgMultipartFailed: {
		"en": "Could not parse multipart form",
		"zh": "无法解析 multipart 表单",
		"ja": "multipart フォームを解析できませんでした",
	},
	msgReadUpload: {
		"en": "Could not read uploaded file",
		"zh": "无法读取上传的文件",
		"ja": "アップロードされたファイルを読み取れませんでした",
	},
	msgBufferFailed: {
		"en": "Could not buffer upload",
		"zh": "无法缓存上传内容",
		"ja": "アップロードを一時保存できませんでした",
	},
	msgResetCache: {
		"en": "Cannot reset cache file",
		"zh": "无法重置缓存文件",
		"ja": "一時ファイルを巻き戻せませんでした",
	},
	msgEmptyUpload: {
		"en": "Could not upload empty file",
		"zh": "不能上传空文件",
		"ja": "空のファイルはアップロードできません",
	},
	msgPrescanFailed: {
		"en": "Could not perform prescan",
		"zh": "无法执行病毒预扫描",
		"ja": "ウイルスの事前スキャンを実行できませんでした",
	},
	msgVirusFound: {
		"en": "Clamav prescan found a virus",
		"zh": "ClamAV 预扫描发现病毒",
		"ja": "ClamAV の事前スキャンでウイルスが検出されました",
	},
	msgNoVirusTotal: {
		"en": "VirusTotal is not configured on this server",
		"zh": "本服务器未配置 VirusTotal",
		"ja": "このサーバーでは VirusTotal が設定されていません",
	},
	msgMaxDownloads: {
		"en": "Max-Downloads must be a positive integer",
		"zh": "Max-Downloads 必须是正整数",
		"ja": "Max-Downloads は正の整数である必要があります",
	},
	msgMaxDays: {
		"en": "Max-Days must be a positive integer",
		"zh": "Max-Days 必须是正整数",
		"ja": "Max-Days は正の整数である必要があります",
	},
	msgMaxDaysTooLarge: {
		"en": "Max-Days must be %d or less",
		"zh": "Max-Days 不能超过 %d",
		"ja": "Max-Days は %d 以下である必要があります",
	},
	msgContentRangeForm: {
		"en": "Content-Range must look like `bytes <start>-<end>/<total>`",
		"zh": "Content-Range 的格式必须为 `bytes <start>-<end>/<total>`",
		"ja": "Content-Range は `bytes <start>-<end>/<total>` の形式である必要があります",
	},
	msgContentRangeFrom: {
		"en": "Content-Range start is out of range",
		"zh": "Content-Range 的起始位置超出范围",
		"ja": "Content-Range の開始位置が範囲外です",
	},
	msgContentRangeTo: {
		"en": "Content-Range end is out of range",
		"zh": "Content-Range 的结束位置超出范围",
		"ja": "Content-Range の終了位置が範囲外です",
	},
	msgContentRangeAll: {
		"en": "Content-Range total is out of range",
		"zh": "Content-Range 的总大小超出范围",
		"ja": "Content-Range の合計サイズが範囲外です",
	},
	msgContentRangeSpan: {
		"en": "Content-Range is not a valid span of the declared total",
		"zh": "Content-Range 不是所声明总大小内的有效区间",
		"ja": "Content-Range が宣言された合計サイズの有効な範囲ではありません",
	},
}

// translate renders one message. An unknown ID returns its own name rather
// than an empty body, so a missing entry is visible instead of silent.
func translate(lang string, id msgID, args ...any) string {
	variants, ok := messages[id]
	if !ok {
		return string(id)
	}

	text, ok := variants[lang]
	if !ok {
		text = variants[defaultLang]
	}

	if len(args) == 0 {
		return text
	}

	return fmt.Sprintf(text, args...)
}

// negotiateLang picks the best supported language for a request.
//
// Only the primary subtag is compared, so zh-Hans-SG and zh-TW both resolve to
// the one Chinese catalogue — this is a handful of error sentences, not a
// localisation product, and offering a regional variant that does not exist
// would just mean falling through to English.
func negotiateLang(r *http.Request) string {
	if r == nil {
		return defaultLang
	}

	type candidate struct {
		lang    string
		quality float64
		order   int
	}

	var candidates []candidate

	for i, part := range strings.Split(r.Header.Get("Accept-Language"), ",") {
		fields := strings.Split(strings.TrimSpace(part), ";")
		tag := strings.ToLower(strings.TrimSpace(fields[0]))
		if tag == "" {
			continue
		}

		quality := 1.0
		for _, field := range fields[1:] {
			field = strings.TrimSpace(field)
			if !strings.HasPrefix(field, "q=") {
				continue
			}
			if q, err := strconv.ParseFloat(strings.TrimPrefix(field, "q="), 64); err == nil {
				quality = q
			}
		}

		if quality <= 0 {
			continue
		}

		candidates = append(candidates, candidate{lang: tag, quality: quality, order: i})
	}

	// A stable sort on the listed order keeps equal-quality tags in the order
	// the client wrote them, which is what the client meant by writing them.
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].quality > candidates[j].quality
	})

	for _, c := range candidates {
		primary := c.lang
		if i := strings.IndexByte(primary, '-'); i >= 0 {
			primary = primary[:i]
		}

		if primary == "*" {
			return defaultLang
		}
		if _, ok := messages[msgNotFound][primary]; ok {
			return primary
		}
	}

	return defaultLang
}

// httpErrorMsg writes a translated error body. It takes no server because the
// panic-recovery middleware runs outside one.
func httpErrorMsg(w http.ResponseWriter, r *http.Request, status int, id msgID, args ...any) {
	lang := negotiateLang(r)

	w.Header().Set("Content-Language", lang)
	addVary(w, "Accept-Language")

	http.Error(w, translate(lang, id, args...), status)
}

// httpError is httpErrorMsg on a server, which is how every handler reaches it.
func (s *Server) httpError(w http.ResponseWriter, r *http.Request, status int, id msgID, args ...any) {
	httpErrorMsg(w, r, status, id, args...)
}

// httpErrorFor writes a translated body when err carries a message ID, and
// falls back to the error's own text otherwise — which is what internal
// failures should keep saying.
func (s *Server) httpErrorFor(w http.ResponseWriter, r *http.Request, status int, err error) {
	var ue *userError
	if errors.As(err, &ue) {
		s.httpError(w, r, status, ue.id, ue.args...)
		return
	}

	http.Error(w, err.Error(), status)
}

// userError is a validation failure whose text a user is meant to read, so it
// travels as a message ID rather than as a finished English sentence.
type userError struct {
	id   msgID
	args []any
}

func (e *userError) Error() string {
	return translate(defaultLang, e.id, e.args...)
}

func userErrorf(id msgID, args ...any) error {
	return &userError{id: id, args: args}
}

// addVary appends a field to Vary without dropping what is already there.
func addVary(w http.ResponseWriter, field string) {
	existing := w.Header().Get("Vary")
	if existing == "" {
		w.Header().Set("Vary", field)
		return
	}

	for _, part := range strings.Split(existing, ",") {
		if strings.EqualFold(strings.TrimSpace(part), field) {
			return
		}
	}

	w.Header().Set("Vary", existing+", "+field)
}
