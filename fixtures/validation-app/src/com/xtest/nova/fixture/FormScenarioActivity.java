package com.xtest.nova.fixture;

import android.app.Activity;
import android.os.Bundle;
import android.text.InputType;
import android.text.method.PasswordTransformationMethod;
import android.view.View;
import android.widget.Button;
import android.widget.CheckBox;
import android.widget.EditText;
import android.widget.LinearLayout;
import android.widget.ScrollView;
import android.widget.TextView;

public final class FormScenarioActivity extends Activity {
    private int dynamicCount;

    @Override public void onCreate(Bundle state) {
        super.onCreate(state);
        ScrollView scroll = new ScrollView(this);
        LinearLayout content = new LinearLayout(this);
        content.setOrientation(LinearLayout.VERTICAL);
        content.setPadding(24, 24, 24, 24);
        TextView status = label("表单状态：未提交", "form-status");
        EditText plain = field("普通文本", "form-plain", InputType.TYPE_CLASS_TEXT);
        plain.setText("杭州 Nova");
        EditText number = field("边界数字", "form-number", InputType.TYPE_CLASS_NUMBER | InputType.TYPE_NUMBER_FLAG_SIGNED);
        number.setText("-2147483648");
        EditText email = field("邮箱", "form-email", InputType.TYPE_CLASS_TEXT | InputType.TYPE_TEXT_VARIATION_EMAIL_ADDRESS);
        email.setText("nova+fixture@example.test");
        EditText password = field("密码字段不得录制", "form-password", InputType.TYPE_CLASS_TEXT | InputType.TYPE_TEXT_VARIATION_PASSWORD);
        // Some vendor accessibility implementations do not infer the password
        // flag from inputType alone. Set the transformation explicitly so the
        // fixture exercises the Agent's real password-protection path.
        password.setTransformationMethod(PasswordTransformationMethod.getInstance());
        password.setText("secret-fixture");
        CheckBox consent = new CheckBox(this);
        consent.setText("同意测试条款");
        consent.setContentDescription("form-consent");
        Button submit = button("提交表单", "form-submit");
        submit.setOnClickListener(new View.OnClickListener() {
            @Override public void onClick(View view) {
                status.setText("表单状态：已提交；文本=" + plain.getText() + "；数字=" + number.getText() + "；同意=" + consent.isChecked());
            }
        });
        Button duplicateA = button("重复操作", "duplicate-action-a");
        Button duplicateB = button("重复操作", "duplicate-action-b");
        duplicateA.setOnClickListener(new View.OnClickListener() { @Override public void onClick(View view) { status.setText("表单状态：重复操作 A"); } });
        duplicateB.setOnClickListener(new View.OnClickListener() { @Override public void onClick(View view) { status.setText("表单状态：重复操作 B"); } });
        Button dynamic = button("添加动态节点", "form-add-dynamic");
        dynamic.setOnClickListener(new View.OnClickListener() {
            @Override public void onClick(View view) {
                dynamicCount++;
                TextView added = label("动态节点 " + dynamicCount, "dynamic-node-" + dynamicCount);
                content.addView(added, Math.max(1, content.getChildCount() - 1));
                status.setText("表单状态：动态节点=" + dynamicCount);
            }
        });
        Button disabled = button("禁用操作", "form-disabled");
        disabled.setEnabled(false);
        content.addView(status);
        content.addView(plain);
        content.addView(number);
        content.addView(email);
        content.addView(password);
        content.addView(consent);
        content.addView(submit);
        content.addView(duplicateA);
        content.addView(duplicateB);
        content.addView(dynamic);
        content.addView(disabled);
        for (int index = 1; index <= 16; index++) content.addView(label("表单滚动项 " + index, "form-row-" + index));
        scroll.addView(content);
        setContentView(scroll);
        if ("password".equals(getIntent().getStringExtra("focus"))) password.requestFocus();
        else plain.requestFocus();
    }

    private EditText field(String hint, String description, int inputType) {
        EditText value = new EditText(this);
        value.setHint(hint);
        value.setContentDescription(description);
        value.setInputType(inputType);
        value.setSingleLine(true);
        return value;
    }

    private Button button(String text, String description) {
        Button value = new Button(this);
        value.setText(text);
        value.setContentDescription(description);
        return value;
    }

    private TextView label(String text, String description) {
        TextView value = new TextView(this);
        value.setText(text);
        value.setContentDescription(description);
        value.setTextSize(18);
        value.setPadding(20, 24, 20, 24);
        return value;
    }
}
