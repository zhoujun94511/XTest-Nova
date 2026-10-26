package com.openatx.xtest.nova.runner;

import java.util.Arrays;
import java.util.HashSet;
import java.util.Locale;
import java.util.Set;

/** Conservative filter for irreversible or externally fulfilled generic actions. */
final class SafetyActionPolicy {
    private static final Set<String> LANGUAGE_SETTING_LABELS = new HashSet<>(Arrays.asList(
            "language", "languages", "locale",
            "app language", "display language", "change language", "switch language", "system language", "select language",
            "语言", "語言", "语言设置", "語言設置", "语言设定", "語言設定", "切换语言", "切換語言",
            "应用语言", "應用語言", "显示语言", "顯示語言", "多语言", "多語言", "选择语言", "選擇語言"
    ));

    private static final Set<String> LANGUAGE_OPTION_LABELS = new HashSet<>(Arrays.asList(
            "english", "chinese", "spanish", "french", "german", "korean", "russian",
            "portuguese", "arabic", "hindi", "vietnamese", "indonesian", "malay",
            "turkish", "italian", "polish", "dutch", "thai", "ukrainian", "czech",
            "swedish", "danish", "finnish", "norwegian", "romanian", "hungarian",
            "greek", "hebrew", "japanese",
            "中文", "简体中文", "简体", "繁体中文", "繁体", "繁體中文", "繁體", "简體中文", "简體",
            "英语", "英文", "汉语", "漢語", "日语", "日語", "日本語", "韩语", "韓語", "한국어",
            "español", "français", "deutsch", "русский", "português", "العربية", "हिन्दी",
            "tiếng việt", "bahasa indonesia", "bahasa melayu", "türkçe", "italiano",
            "polski", "nederlands", "ไทย", "ภาษาไทย", "українська", "čeština", "svenska",
            "dansk", "suomi", "norsk", "română", "magyar", "ελληνικά", "עברית"
    ));

    private static final String[] INSTALL_OR_DOWNLOAD_MARKERS = {
            "install", "download", "get app", "play store", "app store",
            "安装", "下载", "获取应用", "应用商店"
    };

    private SafetyActionPolicy() {}

    static boolean blocksGenericAction(String text, String description, String resourceId) {
        String candidate = ((text == null ? "" : text) + " " +
                (description == null ? "" : description) + " " +
                (resourceId == null ? "" : resourceId)).toLowerCase(Locale.ROOT);
        for (String marker : INSTALL_OR_DOWNLOAD_MARKERS) {
            if (candidate.contains(marker)) return true;
        }
        return false;
    }

    static String activitySimpleName(String activity) {
        String name = activity == null ? "" : activity.toLowerCase(Locale.ROOT).trim();
        int slash = name.lastIndexOf('/');
        if (slash >= 0) name = name.substring(slash + 1);
        while (name.startsWith(".")) name = name.substring(1);
        int dot = name.lastIndexOf('.');
        if (dot >= 0) name = name.substring(dot + 1);
        return name;
    }

    static String resourceEntry(String resourceId) {
        String value = resourceId == null ? "" : resourceId.toLowerCase(Locale.ROOT).trim();
        int slash = value.lastIndexOf('/');
        if (slash >= 0) return value.substring(slash + 1);
        int colon = value.lastIndexOf(':');
        if (colon >= 0) return value.substring(colon + 1);
        return value;
    }

    static boolean isLanguageSettingsActivity(String activity) {
        String name = activitySimpleName(activity);
        if (name.contains("launcher")) return false;
        return name.contains("language") || name.contains("locale");
    }

    static String normalizeLabel(String value) {
        if (value == null) return "";
        return value.toLowerCase(Locale.ROOT).trim().replaceAll("\\s+", " ");
    }

    /** Display-language controls stay out of exploration. Scene keywords cover Chinese and English only. */
    static boolean blocksLanguageSwitch(String text, String description, String resourceId) {
        if (isLanguageOptionLabel(text) || isLanguageOptionLabel(description)) return false;
        String entry = resourceEntry(resourceId);
        if (entry.contains("language") || entry.contains("locale") || entry.contains("i18n")
                || entry.contains("lang_") || entry.contains("_lang") || "lang".equals(entry)) {
            return true;
        }
        return LANGUAGE_SETTING_LABELS.contains(normalizeLabel(text))
                || LANGUAGE_SETTING_LABELS.contains(normalizeLabel(description));
    }

    static boolean blocksLanguagePageAction(String activity, String actionType, boolean scroll) {
        if (scroll || "back".equals(actionType)) return false;
        return isLanguageSettingsActivity(activity);
    }

    static boolean isLanguageOptionLabel(String value) {
        return LANGUAGE_OPTION_LABELS.contains(normalizeLabel(value));
    }

    static boolean isLanguagePickerConfirm(String value) {
        switch (normalizeLabel(value)) {
            case "continue":
            case "next":
            case "skip":
            case "get started":
            case "start exploring":
            case "ok":
            case "okay":
            case "confirm":
            case "done":
            case "apply":
            case "继续":
            case "下一步":
            case "跳过":
            case "确定":
            case "确认":
            case "完成":
                return true;
            default:
                return false;
        }
    }

    static boolean isBackControl(String text, String description, String resourceId) {
        String entry = resourceEntry(resourceId);
        if (entry.contains("back_iv") || entry.contains("navigation_iv") || "back".equals(entry)) return true;
        String label = normalizeLabel(text);
        if (label.isEmpty()) label = normalizeLabel(description);
        return "back".equals(label) || "navigate up".equals(label) || "返回".equals(label) || "返回上一页".equals(label);
    }

    static boolean isStartupLanguageSurface(String activity) {
        String name = activitySimpleName(activity);
        if (name.contains("launcher")) return false;
        return !name.contains("setting") && !name.contains("language") && !name.contains("locale");
    }

    static int startupLanguageRank(String value) {
        switch (normalizeLabel(value)) {
            case "english":
                return 2;
            case "中文":
            case "简体中文":
            case "简體中文":
            case "简体":
            case "简體":
                return 1;
            default:
                return 0;
        }
    }

    static int bestStartupLanguageRank(java.util.List<String[]> taps) {
        int rank = 0;
        if (taps == null) return 0;
        for (String[] tap : taps) {
            if (tap == null) continue;
            if (tap.length > 0) rank = Math.max(rank, startupLanguageRank(tap[0]));
            if (tap.length > 1) rank = Math.max(rank, startupLanguageRank(tap[1]));
        }
        return rank;
    }

    static boolean isLanguagePickerNeutral(String text, String description, String resourceId) {
        if (isBackControl(text, description, resourceId)) return true;
        if (normalizeLabel(text).isEmpty() && normalizeLabel(description).isEmpty()) return true;
        return LANGUAGE_SETTING_LABELS.contains(normalizeLabel(text))
                || LANGUAGE_SETTING_LABELS.contains(normalizeLabel(description));
    }

    static boolean hasLanguageResource(String resourceId) {
        String entry = resourceEntry(resourceId);
        return entry.contains("language") || entry.contains("locale") || entry.contains("i18n")
                || entry.contains("lang_") || entry.contains("_lang") || "lang".equals(entry);
    }

    static int[] languagePageCensus(java.util.List<String[]> taps) {
        int options = 0;
        int other = 0;
        if (taps != null) {
            for (String[] tap : taps) {
                if (tap == null) continue;
                String tapText = tap.length > 0 && tap[0] != null ? tap[0] : "";
                String tapDescription = tap.length > 1 && tap[1] != null ? tap[1] : "";
                String tapResource = tap.length > 2 && tap[2] != null ? tap[2] : "";
                if (isLanguageOptionLabel(tapText) || isLanguageOptionLabel(tapDescription)) {
                    options++;
                    continue;
                }
                if (isLanguagePickerNeutral(tapText, tapDescription, tapResource) || isLanguagePickerConfirm(tapText) || isLanguagePickerConfirm(tapDescription)) continue;
                other++;
            }
        }
        return new int[] {options, other};
    }

    /** A language name whose resource entry is a language id stays blocked outside the selected startup choice. */
    static boolean blocksNamedLanguageResource(java.util.List<String[]> taps, String activity, String text, String description, String resourceId, boolean scroll, boolean back) {
        if (scroll || back || isBackControl(text, description, resourceId)) return false;
        if (!isLanguageOptionLabel(text) && !isLanguageOptionLabel(description)) return false;
        if (!hasLanguageResource(resourceId)) return false;
        if (!isStartupLanguageSurface(activity) || !isLanguagePickerPage(taps)) return true;
        int mine = Math.max(startupLanguageRank(text), startupLanguageRank(description));
        return mine == 0 || mine != bestStartupLanguageRank(taps);
    }

    static boolean isLanguagePickerPage(java.util.List<String[]> taps) {
        int[] census = languagePageCensus(taps);
        return census[0] >= 2 && census[1] == 0;
    }

    /**
     * A first-launch language page has at least two controls whose whole text is a language name.
     * Confirm and skip labels do not keep it ordinary. A real content control does.
     * On that page the startup choice stays available: English, otherwise 中文. Other taps stay blocked.
     * On a mixed page only the language names are blocked.
     */
    static boolean blocksLanguagePickerTap(java.util.List<String[]> taps, String activity, String text, String description, String resourceId, boolean scroll, boolean back) {
        if (scroll || back || isBackControl(text, description, resourceId)) return false;
        int[] census = languagePageCensus(taps);
        int options = census[0];
        int other = census[1];
        if (options < 2) return false;
        if (other == 0) {
            int mine = Math.max(startupLanguageRank(text), startupLanguageRank(description));
            return !isStartupLanguageSurface(activity) || mine == 0 || mine != bestStartupLanguageRank(taps);
        }
        if (isStartupLanguageSurface(activity) && (isLanguagePickerConfirm(text) || isLanguagePickerConfirm(description))) return true;
        return isLanguageOptionLabel(text) || isLanguageOptionLabel(description);
    }

    static boolean isLanguageChoice(String activity, String text, String description) {
        String name = activitySimpleName(activity);
        if (!name.contains("setting") && !name.contains("language") && !name.contains("locale")) return false;
        return LANGUAGE_OPTION_LABELS.contains(normalizeLabel(text))
                || LANGUAGE_OPTION_LABELS.contains(normalizeLabel(description));
    }

    static void selfTest() {
        if (!blocksGenericAction("安装", "", "")) throw new AssertionError("Chinese install CTA was not blocked");
        if (!blocksGenericAction("", "Download now", "")) throw new AssertionError("download CTA was not blocked");
        if (!blocksGenericAction("", "", "com.example:id/install_button")) throw new AssertionError("install resource was not blocked");
        if (blocksGenericAction("开始游戏", "", "com.example:id/play")) throw new AssertionError("ordinary game action was blocked");
        if (!blocksLanguageSwitch("Language", "", "com.example:id/item")) throw new AssertionError("language setting was explorable");
        if (!blocksLanguageSwitch("", "", "com.example:id/app_locale")) throw new AssertionError("locale resource was explorable");
        if (blocksLanguageSwitch("中文短剧", "", "com.example:id/title")) throw new AssertionError("ordinary title was treated as a language switch");
        if (blocksLanguageSwitch("语言学习", "", "com.example:id/course")) throw new AssertionError("lesson title was treated as a language switch");
        if (blocksLanguageSwitch("body language", "", "com.example:id/idiom")) throw new AssertionError("idiom was treated as a language switch");
        if (blocksLanguageSwitch("Open", "", "com.example.language:id/play")) throw new AssertionError("package name was treated as a language resource");
        if (blocksLanguageSwitch("中文", "", "com.example:id/title")) throw new AssertionError("language name was blocked outside settings");
        if (!isLanguageChoice("com.example/.SettingsActivity", "中文", "") || isLanguageChoice("com.example/.MainActivity", "中文", ""))
            throw new AssertionError("language name was not limited to settings pages");
        if (isLanguageChoice("com.example/.SettingsActivity", "中文短剧", "")) throw new AssertionError("settings title was treated as a language choice");
        if (!isLanguageSettingsActivity("com.example/.LanguageActivity") || isLanguageSettingsActivity("com.example.language/.MainActivity") || isLanguageSettingsActivity("com.example/.MainActivity"))
            throw new AssertionError("language settings activity was not recognized");
        if (!blocksLanguagePageAction("com.example/.LanguageActivity", "tap", false) || blocksLanguagePageAction("com.example/.LanguageActivity", "back", false) || blocksLanguagePageAction("com.example/.LanguageActivity", "tap", true) || blocksLanguagePageAction("com.example/.MainActivity", "tap", false))
            throw new AssertionError("language page still accepted an onboarding tap");
        java.util.List<String[]> launchPicker = java.util.Arrays.asList(
                new String[] {"中文", ""}, new String[] {"English", ""}, new String[] {"继续", ""});
        if (blocksLanguagePickerTap(launchPicker, "com.example/.MainActivity", "English", "", "", false, false)
                || !blocksLanguagePickerTap(launchPicker, "com.example/.MainActivity", "中文", "", "", false, false)
                || !blocksLanguagePickerTap(launchPicker, "com.example/.MainActivity", "继续", "", "", false, false)
                || !blocksLanguagePickerTap(launchPicker, "com.example/.SettingsActivity", "English", "", "", false, false)
                || blocksLanguagePickerTap(launchPicker, "com.example/.MainActivity", "", "", "", true, false))
            throw new AssertionError("first-launch language page did not keep English as the only choice");
        if (blocksLanguageSwitch("English", "", "com.example:id/language_en"))
            throw new AssertionError("startup language name was hidden by its resource");
        if (!blocksNamedLanguageResource(java.util.Arrays.asList(new String[] {"中文", "", "com.example:id/app_language"}, new String[] {"Play", ""}), "com.example/.MainActivity", "中文", "", "com.example:id/app_language", false, false))
            throw new AssertionError("settings language value was tappable from its resource");
        if (blocksNamedLanguageResource(launchPicker, "com.example/.MainActivity", "English", "", "com.example:id/language_en", false, false))
            throw new AssertionError("startup English choice was blocked by its resource");
        java.util.List<String[]> mixed = java.util.Arrays.asList(
                new String[] {"中文", ""}, new String[] {"English", ""}, new String[] {"Play", ""});
        if (!blocksLanguagePickerTap(mixed, "com.example/.MainActivity", "English", "", "", false, false) || blocksLanguagePickerTap(mixed, "com.example/.MainActivity", "Play", "", "", false, false))
            throw new AssertionError("content page with language names blocked an ordinary control");
        java.util.List<String[]> single = java.util.Arrays.asList(new String[] {"中文", ""}, new String[] {"Play", ""});
        if (blocksLanguagePickerTap(single, "com.example/.MainActivity", "中文", "", "", false, false))
            throw new AssertionError("one language name blocked an ordinary page");
        System.out.println("SafetyActionPolicy self-test passed");
    }
}
