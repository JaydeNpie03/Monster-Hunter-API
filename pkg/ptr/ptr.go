package ptr

const IMAGE_URL = "https://141.148.71.142/images/monster"
const ICON_URL = "https://141.148.71.142/images/monster/icons"

func StrToPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func PtrToStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func StrArrayToPtr(s []string) []*string {
	if len(s) == 0 {
		return nil
	}
	ptrs := make([]*string, len(s))
	for i, v := range s {
		ptrs[i] = &v
	}
	return ptrs
}

func CreateImageURL(id string) *string {
	if id == "" {
		return nil
	}

	return StrToPtr(IMAGE_URL + "/" + id + ".png")
}

func CreateIconURL(id string) *string {
        if id == "" {
                return nil
        }

        return StrToPtr(ICON_URL + "/" + id + "_icon.png")
}

func IntToPtr(i int) *int {
	if i == 0 {
		return nil
	}
	return &i
}
